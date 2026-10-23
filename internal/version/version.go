// Package version holds the dev-mcp build identity advertised over MCP.
package version

const (
	// Name is the MCP implementation name.
	Name = "dev-mcp"
)

// Version is the server version returned by ping and by --version.
// Release builds replace it with -X. The default is the development version.
var Version = "0.1.0"

// Flag reports whether args ask the process to print its version and exit.
// --version and -version are recognized. The server does not start in that case.
func Flag(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			return true
		}
	}
	return false
}
