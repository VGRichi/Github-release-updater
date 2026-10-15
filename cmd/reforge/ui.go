package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// cli holds what every command shares: where to read and write, and the list file.
type cli struct {
	stdin       *bufio.Reader
	stdout      io.Writer
	stderr      io.Writer
	interactive bool // someone can answer questions
	configPath  string
	verbose     bool
	refresh     bool // ask GitHub again rather than reuse recent answers
	failed      bool // a command finished, but something in it failed (exit code 1)
}

func newCLI(stdin io.Reader, stdout, stderr io.Writer) *cli {
	c := &cli{stdin: bufio.NewReader(stdin), stdout: stdout, stderr: stderr}
	if f, ok := stdin.(*os.File); ok {
		c.interactive = typedInput(f)
	}
	return c
}

func (c *cli) warn(msg string)     { fmt.Fprintf(c.stderr, "warning: %s\n", msg) }
func (c *cli) progress(msg string) { fmt.Fprintf(c.stderr, "  %s\n", msg) }
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

// table prints rows under headers, in aligned columns.
func (c *cli) table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	lines := make([]string, len(headers))
	for i, h := range headers {
		lines[i] = strings.Repeat("-", len(h))
	}
	fmt.Fprintln(w, strings.Join(lines, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

func (c *cli) json(v any) error {
	enc := json.NewEncoder(c.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
