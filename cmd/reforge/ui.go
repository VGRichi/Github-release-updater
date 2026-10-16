package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/VGRichi/Reforge/internal/core"
	"github.com/VGRichi/Reforge/internal/github"
)

// cli holds what every command shares: where to read and write, and the list file.
type cli struct {
	stdin       *bufio.Reader
	stdout      io.Writer
	stderr      io.Writer
	interactive bool // someone can answer questions
	configPath  string
	verbose     bool
	failed      bool // a command finished, but something in it failed (exit code 1)
	color       bool // stdout is a terminal that shows colours
	colorErr    bool // the same for stderr
	eng         *core.Engine
	cache       *github.Cache
}

func newCLI(stdin io.Reader, stdout, stderr io.Writer) *cli {
	c := &cli{stdin: bufio.NewReader(stdin), stdout: stdout, stderr: stderr}
	if f, ok := stdout.(*os.File); ok {
		c.color = terminalColors(f)
	}
	if f, ok := stderr.(*os.File); ok {
		c.colorErr = terminalColors(f)
	}
	if f, ok := stdin.(*os.File); ok {
		c.interactive = typedInput(f)
	}
	return c
}

func (c *cli) warn(msg string) {
	fmt.Fprintf(c.stderr, "%s %s\n", paint(c.colorErr, yellow, "warning:"), msg)
}
func (c *cli) progress(msg string) { fmt.Fprintln(c.stderr, paint(c.colorErr, cyan, "  "+msg)) }

// say prints a line to stderr in a colour, like "Checking 6 apps..." in cyan.
func (c *cli) say(code, msg string) { fmt.Fprintln(c.stderr, paint(c.colorErr, code, msg)) }

// status is a status in words, coloured by what it means: green when there's nothing to do, yellow
// when there's an update, red for an error.
func (c *cli) status(s core.Status, text string) string {
	switch s {
	case core.UpToDate, core.Updated, core.Installed:
		return paint(c.color, green, text)
	case core.UpdateAvailable, core.NeedsAdmin:
		return paint(c.color, yellow, text)
	case core.Failed:
		return paint(c.color, red, text)
	}
	return paint(c.color, dim, text)
}
func (c *cli) detail(msg string) {
	if c.verbose {
		fmt.Fprintf(c.stderr, "  %s\n", msg)
	}
}

// Choose asks a question in the terminal. With nobody to answer (a scheduled task, input from a pipe)
// it takes the default, which is always the safe answer.
func (c *cli) Choose(question string, answers []string, def int) int {
	if !c.interactive {
		c.warn(fmt.Sprintf("can't ask here: %s Answering '%s'.", question, answers[def]))
		return def
	}
	var options []string
	for i, a := range answers {
		opt := fmt.Sprintf("[%c] %s", a[0], a)
		if i == def {
			opt += " (default)"
		}
		options = append(options, opt)
	}
	for {
		fmt.Fprintf(c.stderr, "%s\n%s: ", question, strings.Join(options, "  "))
		line, err := c.stdin.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" {
			if err != nil { // the input ended: nobody is there to answer the next questions either
				fmt.Fprintln(c.stderr)
				c.interactive = false
			}
			return def
		}
		for i, a := range answers {
			if line == strings.ToLower(a) || line == strings.ToLower(a[:1]) {
				return i
			}
		}
	}
}

// table prints rows under headers, in columns aligned by what's visible (colour codes take no space).
func (c *cli) table(headers []string, rows [][]string) {
	lines := make([]string, len(headers))
	for i, h := range headers {
		lines[i] = strings.Repeat("-", len(h))
	}
	all := append([][]string{headers, lines}, rows...)
	widths := make([]int, len(headers))
	for _, row := range all {
		for i, cell := range row {
			widths[i] = max(widths[i], visibleWidth(cell))
		}
	}
	for _, row := range all {
		var b strings.Builder
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-visibleWidth(cell)+2))
			}
		}
		fmt.Fprintln(c.stdout, strings.TrimRight(b.String(), " "))
	}
}

func (c *cli) json(v any) error {
	enc := json.NewEncoder(c.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
