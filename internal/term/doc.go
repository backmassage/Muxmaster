// Package term provides ANSI color state and terminal detection.
// It exports color variables (Cyan, Green, Red, etc.) that are set
// once by Configure and read at log-write time by the logging package.
//
// Files:
//   - term.go:        Configure, IsTerminal, color variable exports
package term
