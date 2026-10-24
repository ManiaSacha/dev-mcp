// Package logs reads local application logs and returns them with secrets masked.
//
// The tools read regular files only. They do not start a process and they
// do not invoke a shell. A large file is scanned from the end, up to a
// fixed byte window, so a multi-gigabyte log cannot be loaded whole.
package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/redaction"
)

const (
	// ToolTail returns the end of a log file.
	ToolTail = "logs_tail"
	// ToolSearch finds literal matches in a log file.
	ToolSearch = "logs_search"
	// ToolRedact masks secrets in caller-supplied text.
	ToolRedact = "logs_redact"

	defaultTailLines  = 200
	maxTailLines      = 2000
	defaultMatchLimit = 100
	maxMatchLimit     = 500
	defaultScanBytes  = 8 << 20
	maxLineBytes      = 16 << 10
	maxRedactBytes    = 1 << 20

	originFile   = "file"
	originWindow = "window"
)

// Handler reads log files. The zero value scans at most 8 MiB from the end.
type Handler struct {
	// maxScanBytes limits how much of a file is read, from the end.
	// Tests set a smaller window. Zero uses defaultScanBytes.
	maxScanBytes int
}

// NewHandler returns a handler with the production scan window.
func NewHandler() Handler {
	return Handler{maxScanBytes: defaultScanBytes}
}

// TailArgs selects a log file and how many lines to return.
type TailArgs struct {
	Path  string `json:"path" jsonschema:"path of a regular log file"`
	Lines int    `json:"lines,omitempty" jsonschema:"lines to return from the end of the scanned window. Defaults to 200. Maximum 2000."`
}

// SearchArgs selects a log file and a literal query.
type SearchArgs struct {
	Path       string `json:"path" jsonschema:"path of a regular log file"`
	Query      string `json:"query" jsonschema:"literal text to find, matched against the redacted text so secret values cannot be searched for. Regular expression syntax is not interpreted."`
	Matches    int    `json:"matches,omitempty" jsonschema:"maximum matching lines to return. Defaults to 100. Maximum 500."`
	IgnoreCase *bool  `json:"ignoreCase,omitempty" jsonschema:"compare the query without case when true"`
}

// RedactArgs is text to mask. The text is not written to disk.
type RedactArgs struct {
	Text string `json:"text" jsonschema:"log text to mask. Maximum 1 MiB."`
}

// LogLine is one redacted line.
type LogLine struct {
	Number int    `json:"number" jsonschema:"1-based line number. Absolute in the file when lineOrigin is file, otherwise the number within the scanned window."`
	Text   string `json:"text" jsonschema:"line text after redaction"`
}

// TailResult is the redacted end of a log file.
type TailResult struct {
	Path         string    `json:"path" jsonschema:"path that was read"`
	Lines        []LogLine `json:"lines" jsonschema:"redacted lines, oldest first"`
	LineCount    int       `json:"lineCount" jsonschema:"number of lines returned"`
	Truncated    bool      `json:"truncated" jsonschema:"true when the file was larger than the scan window"`
	LineOrigin   string    `json:"lineOrigin" jsonschema:"file when line numbers are absolute, window when the scan started at the end of a large file"`
	Redactions   int       `json:"redactions" jsonschema:"number of secrets replaced"`
	ScannedBytes int       `json:"scannedBytes" jsonschema:"bytes read from the end of the file"`
}

// SearchResult is the redacted matches from a log file.
type SearchResult struct {
	Path              string    `json:"path" jsonschema:"path that was read"`
	Matches           []LogLine `json:"matches" jsonschema:"redacted matching lines, oldest first"`
	MatchCount        int       `json:"matchCount" jsonschema:"number of matches returned"`
	MatchLimitReached bool      `json:"matchLimitReached" jsonschema:"true when more matches existed than the limit"`
	Truncated         bool      `json:"truncated" jsonschema:"true when the file was larger than the scan window"`
	LineOrigin        string    `json:"lineOrigin" jsonschema:"file when line numbers are absolute, window when the scan started at the end of a large file"`
	Redactions        int       `json:"redactions" jsonschema:"number of secrets replaced"`
	ScannedBytes      int       `json:"scannedBytes" jsonschema:"bytes read from the end of the file"`
}

// RedactResult is masked text.
type RedactResult struct {
	Text       string `json:"text" jsonschema:"text with secrets replaced by [REDACTED]"`
	Redactions int    `json:"redactions" jsonschema:"number of secrets replaced"`
}

// Tail returns the redacted end of a log file.
func (h Handler) Tail(ctx context.Context, _ *mcp.CallToolRequest, args TailArgs) (*mcp.CallToolResult, TailResult, error) {
	var zero TailResult
	n, err := bounded(args.Lines, defaultTailLines, maxTailLines, "lines")
	if err != nil {
		return nil, zero, toolErr(ToolTail, err)
	}
	scanned, err := h.read(ctx, args.Path)
	if err != nil {
		return nil, zero, toolErr(ToolTail, err)
	}
	start := 0
	if len(scanned.lines) > n {
		start = len(scanned.lines) - n
	}
	lines, redactions := maskLines(scanned.lines[start:], start)
	return nil, TailResult{
		Path:         args.Path,
		Lines:        lines,
		LineCount:    len(lines),
		Truncated:    scanned.truncated,
		LineOrigin:   scanned.origin,
		Redactions:   redactions,
		ScannedBytes: scanned.scannedBytes,
	}, nil
}

// Search returns redacted lines that contain the query as literal text.
// The query is matched against the redacted text, not the original line.
func (h Handler) Search(ctx context.Context, _ *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error) {
	var zero SearchResult
	if args.Query == "" || strings.ContainsRune(args.Query, 0) {
		return nil, zero, toolErr(ToolSearch, errors.New("query is empty"))
	}
	if len(args.Query) > 500 {
		return nil, zero, toolErr(ToolSearch, errors.New("query is too long"))
	}
	limit, err := bounded(args.Matches, defaultMatchLimit, maxMatchLimit, "matches")
	if err != nil {
		return nil, zero, toolErr(ToolSearch, err)
	}
	scanned, err := h.read(ctx, args.Path)
	if err != nil {
		return nil, zero, toolErr(ToolSearch, err)
	}

	query := args.Query
	ignoreCase := args.IgnoreCase != nil && *args.IgnoreCase
	if ignoreCase {
		query = strings.ToLower(query)
	}
	// Mask the whole scan first so multi-line secrets are caught.
	maskedLines, _ := redaction.RedactLines(scanned.lines)
	matches := make([]LogLine, 0)
	redactions := 0
	limited := false
	for i, line := range scanned.lines {
		// Match the masked text so a query cannot confirm a secret's value.
		haystack := maskedLines[i]
		if ignoreCase {
			haystack = strings.ToLower(haystack)
		}
		if !strings.Contains(haystack, query) {
			continue
		}
		if len(matches) == limit {
			limited = true
			break
		}
		matches = append(matches, LogLine{Number: i + 1, Text: redaction.Clip(maskedLines[i], maxLineBytes)})
		if maskedLines[i] != line {
			redactions++
		}
	}
	return nil, SearchResult{
		Path:              args.Path,
		Matches:           matches,
		MatchCount:        len(matches),
		MatchLimitReached: limited,
		Truncated:         scanned.truncated,
		LineOrigin:        scanned.origin,
		Redactions:        redactions,
		ScannedBytes:      scanned.scannedBytes,
	}, nil
}

// Redact masks secrets in text supplied by the caller.
func (h Handler) Redact(ctx context.Context, _ *mcp.CallToolRequest, args RedactArgs) (*mcp.CallToolResult, RedactResult, error) {
	var zero RedactResult
	if err := ctx.Err(); err != nil {
		return nil, zero, toolErr(ToolRedact, err)
	}
	if len(args.Text) > maxRedactBytes {
		return nil, zero, toolErr(ToolRedact, errors.New("text is too long"))
	}
	masked, n := redaction.Redact(args.Text)
	return nil, RedactResult{Text: masked, Redactions: n}, nil
}

type fileScan struct {
	lines        []string
	origin       string
	truncated    bool
	scannedBytes int
}

func (h Handler) read(ctx context.Context, path string) (scan fileScan, err error) {
	if err = ctx.Err(); err != nil {
		return fileScan{}, err
	}
	if path == "" || strings.ContainsRune(path, 0) || strings.ContainsAny(path, "\r\n") {
		return fileScan{}, errors.New("path is empty or contains a newline")
	}
	f, err := os.Open(path)
	if err != nil {
		return fileScan{}, fmt.Errorf("path: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return fileScan{}, err
	}
	if !info.Mode().IsRegular() {
		return fileScan{}, fmt.Errorf("path %s is not a regular file", path)
	}
	if err = ctx.Err(); err != nil {
		return fileScan{}, err
	}

	size := info.Size()
	if size == 0 {
		return fileScan{lines: []string{}, origin: originFile}, nil
	}
	window := h.scanLimit()
	start := int64(0)
	truncated := false
	if size > int64(window) {
		start = size - int64(window)
		truncated = true
	}
	buf := make([]byte, size-start)
	n, err := f.ReadAt(buf, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return fileScan{}, err
	}
	buf = buf[:n]
	if truncated {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	origin := originFile
	if truncated {
		origin = originWindow
	}
	return fileScan{
		lines:        splitLines(buf),
		origin:       origin,
		truncated:    truncated,
		scannedBytes: n,
	}, nil
}

func (h Handler) scanLimit() int {
	if h.maxScanBytes > 0 {
		return h.maxScanBytes
	}
	return defaultScanBytes
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return []string{}
	}
	if b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	if len(b) > 0 && b[len(b)-1] == '\r' {
		b = b[:len(b)-1]
	}
	raw := bytes.Split(b, []byte{'\n'})
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		lines = append(lines, string(line))
	}
	return lines
}

func maskLines(lines []string, start int) ([]LogLine, int) {
	masked, redactions := redaction.RedactLines(lines)
	out := make([]LogLine, 0, len(lines))
	for i, text := range masked {
		out = append(out, LogLine{Number: start + i + 1, Text: redaction.Clip(text, maxLineBytes)})
	}
	return out, redactions
}

func bounded(n, fallback, max int, name string) (int, error) {
	if n == 0 {
		return fallback, nil
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	if n > max {
		return 0, fmt.Errorf("%s must be at most %d", name, max)
	}
	return n, nil
}

func toolErr(tool string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", tool, err)
}
