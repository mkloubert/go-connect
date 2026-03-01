// Copyright © 2026 Marcel Joachim Kloubert <marcel@kloubert.dev>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package ui

import (
	"fmt"
	"io"
	"os"
)

// ANSI color and formatting escape codes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

// UI provides colored, categorized CLI output.
type UI struct {
	w       io.Writer
	color   bool
	verbose bool
	quiet   bool
}

// NewUI creates a new UI instance that writes to the given writer.
// Use this constructor in tests where you want to capture output.
func NewUI(w io.Writer, color, verbose, quiet bool) *UI {
	return &UI{
		w:       w,
		color:   color,
		verbose: verbose,
		quiet:   quiet,
	}
}

// NewDefaultUI creates a new UI instance that writes to os.Stdout.
// If useColor is true, it additionally checks whether stdout is a terminal;
// color is only enabled when both useColor is true and stdout is a TTY.
func NewDefaultUI(useColor, verbose, quiet bool) *UI {
	if useColor {
		useColor = isTerminal()
	}

	return NewUI(os.Stdout, useColor, verbose, quiet)
}

// isTerminal reports whether os.Stdout is connected to a terminal.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

// colorize wraps text with the given ANSI color code if color output is enabled.
func (u *UI) colorize(color, text string) string {
	if !u.color {
		return text
	}

	return color + text + colorReset
}

// Success prints a green success message with a checkmark symbol.
// Suppressed in quiet mode.
func (u *UI) Success(msg string) {
	if u.quiet {
		return
	}

	fmt.Fprintf(u.w, "  %s\n", u.colorize(colorGreen, "✓ "+msg))
}

// Error prints a red error message with an X symbol.
// Always shown, even in quiet mode.
func (u *UI) Error(msg string) {
	fmt.Fprintf(u.w, "  %s\n", u.colorize(colorRed, "✗ "+msg))
}

// Warning prints a yellow warning message with a warning symbol.
// Suppressed in quiet mode.
func (u *UI) Warning(msg string) {
	if u.quiet {
		return
	}

	fmt.Fprintf(u.w, "  %s\n", u.colorize(colorYellow, "⚠ "+msg))
}

// Info prints an unformatted informational message.
// Suppressed in quiet mode.
func (u *UI) Info(msg string) {
	if u.quiet {
		return
	}

	fmt.Fprintf(u.w, "  %s\n", msg)
}

// Hint prints a hint message with a cyan "Hint:" prefix, preceded by a blank line.
// Suppressed in quiet mode.
func (u *UI) Hint(msg string) {
	if u.quiet {
		return
	}

	fmt.Fprintf(u.w, "\n  %s %s\n", u.colorize(colorCyan, "Hint:"), msg)
}

// Debug prints a gray debug message with a [DEBUG] prefix.
// Only shown when verbose mode is enabled.
func (u *UI) Debug(msg string) {
	if !u.verbose {
		return
	}

	fmt.Fprintf(u.w, "  %s\n", u.colorize(colorGray, "[DEBUG] "+msg))
}

// Header prints a bold header message preceded by a blank line.
// Suppressed in quiet mode.
func (u *UI) Header(msg string) {
	if u.quiet {
		return
	}

	fmt.Fprintf(u.w, "\n  %s\n", u.colorize(colorBold, msg))
}

// Bullet prints a bullet-pointed message with extra indentation.
// Suppressed in quiet mode.
func (u *UI) Bullet(msg string) {
	if u.quiet {
		return
	}

	fmt.Fprintf(u.w, "    • %s\n", msg)
}

// Raw prints a message with no formatting at all.
// Always shown, including in quiet mode. Suitable for script output.
func (u *UI) Raw(msg string) {
	fmt.Fprintf(u.w, "%s\n", msg)
}

// BlankLine prints an empty line.
// Always shown.
func (u *UI) BlankLine() {
	fmt.Fprintln(u.w)
}

// IsVerbose reports whether verbose mode is enabled.
func (u *UI) IsVerbose() bool {
	return u.verbose
}

// IsQuiet reports whether quiet mode is enabled.
func (u *UI) IsQuiet() bool {
	return u.quiet
}
