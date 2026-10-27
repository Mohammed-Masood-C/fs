package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
)

// enableVirtualTerminalProcessing makes the Windows console interpret ANSI
// escape sequences (colors).
const enableVirtualTerminalProcessing = 0x0004

var procSetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")

// painter adds colors to text when writing to a console, and leaves text
// unchanged otherwise (redirected output, service logs, NO_COLOR set).
type painter struct{ color bool }

func newPainter(w io.Writer) painter {
	return painter{color: supportsColor(w)}
}

func supportsColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	h := syscall.Handle(f.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(h, &mode); err != nil {
		return false // not a console
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	ok2, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
	return ok2 != 0
}

func (p painter) wrap(code, s string) string {
	if !p.color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p painter) bold(s string) string   { return p.wrap("1", s) }
func (p painter) dim(s string) string    { return p.wrap("2", s) }
func (p painter) red(s string) string    { return p.wrap("31", s) }
func (p painter) green(s string) string  { return p.wrap("32", s) }
func (p painter) yellow(s string) string { return p.wrap("33", s) }
func (p painter) cyan(s string) string   { return p.wrap("36", s) }

// cell is one table cell: its text, and an optional color applied after the
// column widths are computed (escape codes must not count as width).
type cell struct {
	text  string
	paint func(string) string
}

// printTable prints rows under header, with columns aligned.
func printTable(w io.Writer, p painter, header []string, rows [][]cell) {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, c := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(c.text))
		}
	}

	line := func(cells []cell) {
		var b strings.Builder
		b.WriteString("  ")
		for i, c := range cells {
			text := c.text
			if c.paint != nil {
				text = c.paint(text)
			}
			b.WriteString(text)
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.text)+3))
			}
		}
		fmt.Fprintln(w, b.String())
	}

	head := make([]cell, len(header))
	for i, h := range header {
		head[i] = cell{text: h, paint: p.dim}
	}
	line(head)
	for _, row := range rows {
		line(row)
	}
}
