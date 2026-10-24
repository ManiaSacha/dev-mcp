// Package redaction masks credential-shaped text in log lines.
//
// The replacements keep the surrounding line so a reader can see which
// field was removed. The package does not decrypt or store the original
// secret.
package redaction

import (
	"regexp"
	"unicode/utf8"
)

const mask = "[REDACTED]"

var (
	pemRE = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	urlRE = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:\s/@]+):[^@\s/]+@`)
	// Group 1 is the field name and separator. The value is replaced.
	assignmentRE = regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|secret|api[_-]?key|access[_-]?key|client[_-]?secret|token)\b["']?\s*[=:]\s*)(?:"[^"]*"|'[^']*'|\S+)`)
	bearerRE     = regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
	basicRE      = regexp.MustCompile(`(?i)(authorization\s*:\s*basic\s+)[A-Za-z0-9+/=]{8,}`)
	githubRE     = regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}\b|\bgithub_pat_[A-Za-z0-9_]{20,}\b`)
	slackRE      = regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)
	openaiRE     = regexp.MustCompile(`\bsk-(?:proj-|live-)?[A-Za-z0-9]{20,}\b`)
	stripeRE     = regexp.MustCompile(`\b(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{16,}\b`)
	googleRE     = regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)
	awsRE        = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	jwtRE        = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
)

// Redact returns text with credential-shaped substrings replaced by [REDACTED].
// redactions is the number of replacements. A line that contains no
// recognized secret is returned unchanged.
func Redact(text string) (masked string, redactions int) {
	text, n := replaceWhole(pemRE, text, mask)
	redactions += n
	text, n = replaceTemplate(urlRE, text, `${1}:`+mask+`@`)
	redactions += n
	text, n = replaceTemplate(assignmentRE, text, `${1}`+mask)
	redactions += n
	text, n = replaceTemplate(bearerRE, text, `${1}`+mask)
	redactions += n
	text, n = replaceTemplate(basicRE, text, `${1}`+mask)
	redactions += n
	for _, re := range []*regexp.Regexp{githubRE, slackRE, openaiRE, stripeRE, googleRE, awsRE, jwtRE} {
		text, n = replaceWhole(re, text, mask)
		redactions += n
	}
	return text, redactions
}

func replaceWhole(re *regexp.Regexp, text, repl string) (string, int) {
	n := len(re.FindAllStringIndex(text, -1))
	if n == 0 {
		return text, 0
	}
	return re.ReplaceAllString(text, repl), n
}

func replaceTemplate(re *regexp.Regexp, text, repl string) (string, int) {
	n := len(re.FindAllStringIndex(text, -1))
	if n == 0 {
		return text, 0
	}
	return re.ReplaceAllString(text, repl), n
}

var (
	pemBeginRE = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	pemEndRE   = regexp.MustCompile(`-----END [A-Z0-9 ]*PRIVATE KEY-----`)
)

// RedactLines masks credential-shaped text across a sequence of lines.
// It applies Redact to each line and also masks private-key blocks that
// span several lines. The result has the same number of lines as the
// input so line numbers stay valid. redactions counts a multi-line key
// block once.
func RedactLines(lines []string) (masked []string, redactions int) {
	masked = make([]string, len(lines))
	inKey := false
	for i, line := range lines {
		if inKey {
			masked[i] = mask
			if pemEndRE.MatchString(line) {
				inKey = false
			}
			continue
		}
		if loc := pemBeginRE.FindStringIndex(line); loc != nil && !pemEndRE.MatchString(line[loc[1]:]) {
			masked[i] = line[:loc[0]] + mask
			redactions++
			inKey = true
			continue
		}
		var n int
		masked[i], n = Redact(line)
		redactions += n
	}
	return masked, redactions
}

const truncationMarker = "…[truncated]"

// Clip shortens text to at most max bytes, cutting on a rune boundary, and
// appends a truncation marker when it cut anything. Call it after Redact or
// RedactLines: cutting first can split a secret into pieces too short to
// match a pattern.
func Clip(text string, max int) string {
	if len(text) <= max {
		return text
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + truncationMarker
}
