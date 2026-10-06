// Package output renders results for people (tables, lines) or for programs
// (--json). Commands never print directly; they go through a Printer so the
// two modes stay consistent.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Printer writes to Out for results and Err for diagnostics.
type Printer struct {
	JSON bool
	Out  io.Writer
	Err  io.Writer
}

// JSONValue writes v as indented JSON followed by a newline.
func (p Printer) JSONValue(v any) error {
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Line prints a formatted line to Out (human mode only).
func (p Printer) Line(format string, args ...any) {
	if p.JSON {
		return
	}
	fmt.Fprintf(p.Out, format+"\n", args...)
}

// Note prints a formatted line to Err in both modes; it is for progress and
// hints, never for results.
func (p Printer) Note(format string, args ...any) {
	fmt.Fprintf(p.Err, format+"\n", args...)
}

// Table prints an aligned table (human mode only).
func (p Printer) Table(header []string, rows [][]string) {
	if p.JSON {
		return
	}
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, r := range rows {
		for i := range header {
			if i < len(r) {
				if n := utf8.RuneCountInString(r[i]); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}
	line := func(cells []string) {
		var b strings.Builder
		for i := range header {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			b.WriteString(cell)
			if i < len(header)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+2))
			}
		}
		fmt.Fprintln(p.Out, strings.TrimRight(b.String(), " "))
	}
	line(header)
	for _, r := range rows {
		line(r)
	}
}

// ExitError carries a process exit code with an error.
type ExitError struct {
	Code   int
	Err    error
	Silent bool // the command already printed its report; print nothing more
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Exit wraps err with a specific exit code.
func Exit(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}

// Exitf builds an ExitError from a format string.
func Exitf(code int, format string, args ...any) error {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}

// ExitQuiet sets the exit code without printing anything: for a command that already
// wrote its report (one JSON document, not a second {"error"} object after it).
func ExitQuiet(code int, format string, args ...any) error {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...), Silent: true}
}

// Detailer is implemented by errors that carry structured detail for --json.
type Detailer interface {
	ErrorDetail() any
}

// Fail reports err and returns the exit code to use.
func (p Printer) Fail(err error) int {
	code := 1
	var xe *ExitError
	if errors.As(err, &xe) {
		code = xe.Code
		if xe.Silent {
			return code
		}
	}
	if p.JSON {
		obj := map[string]any{"error": err.Error()}
		var d Detailer
		if errors.As(err, &d) {
			if detail := d.ErrorDetail(); detail != nil {
				obj["detail"] = detail
			}
		}
		_ = p.JSONValue(obj)
	} else {
		fmt.Fprintf(p.Err, "hearthroom: %s\n", err.Error())
	}
	return code
}
