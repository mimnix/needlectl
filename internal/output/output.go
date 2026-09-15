// Package output renders the clean, human-readable progress information
// printed by the needlectl commands. Colour is optional and automatically
// disabled for non-terminal output or when NO_COLOR is set.
package output

import (
	"fmt"
	"io"
	"os"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
)

// Options controls how a Printer renders its output.
type Options struct {
	Color   bool
	Verbose bool
}

// Printer writes structured progress output to a pair of streams.
type Printer struct {
	out     io.Writer
	errOut  io.Writer
	color   bool
	verbose bool
}

// New builds a Printer. out receives regular messages, errOut receives errors.
func New(out, errOut io.Writer, opts Options) *Printer {
	return &Printer{out: out, errOut: errOut, color: opts.Color, verbose: opts.Verbose}
}

// Verbose reports whether debug output is enabled.
func (p *Printer) Verbose() bool { return p.verbose }

// ColorsEnabled decides whether ANSI colour should be used. noColorFlag wins,
// then the NO_COLOR convention, then terminal detection.
func ColorsEnabled(file *os.File, noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	return IsTerminal(file)
}

// IsTerminal reports whether file is an interactive character device.
func IsTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// Step prints a progress step, prefixed by a chevron.
func (p *Printer) Step(format string, args ...any) {
	fmt.Fprintf(p.out, "%s %s\n", p.paint(ansiCyan, "›"), fmt.Sprintf(format, args...))
}

// Success prints a completed step, prefixed by a check mark.
func (p *Printer) Success(format string, args ...any) {
	fmt.Fprintf(p.out, "%s %s\n", p.paint(ansiGreen, "✓"), fmt.Sprintf(format, args...))
}

// Warn prints a non-fatal warning, prefixed by an exclamation mark.
func (p *Printer) Warn(format string, args ...any) {
	fmt.Fprintf(p.errOut, "%s %s\n", p.paint(ansiYellow, "!"), fmt.Sprintf(format, args...))
}

// Error prints an error, prefixed by a cross.
func (p *Printer) Error(format string, args ...any) {
	fmt.Fprintf(p.errOut, "%s %s\n", p.paint(ansiRed, "✗"), fmt.Sprintf(format, args...))
}

// Info prints an indented informational line.
func (p *Printer) Info(format string, args ...any) {
	fmt.Fprintf(p.out, "  %s\n", fmt.Sprintf(format, args...))
}

// Debug prints a line only when verbose output is enabled.
func (p *Printer) Debug(format string, args ...any) {
	if !p.verbose {
		return
	}
	fmt.Fprintf(p.out, "  %s\n", p.paint(ansiDim, fmt.Sprintf(format, args...)))
}

// Field prints an aligned "label: value" line used in summaries.
func (p *Printer) Field(label, value string) {
	padded := fmt.Sprintf("%-12s", label+":")
	if p.color {
		padded = ansiBold + padded + ansiReset
	}
	fmt.Fprintf(p.out, "  %s %s\n", padded, value)
}

// Raw writes a message without any decoration.
func (p *Printer) Raw(format string, args ...any) {
	fmt.Fprintf(p.out, format+"\n", args...)
}

func (p *Printer) paint(code, text string) string {
	if !p.color {
		return text
	}
	return code + text + ansiReset
}
