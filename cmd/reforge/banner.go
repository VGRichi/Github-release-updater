package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// The app icon in text: an orange arrow over an anvil.
var (
	logoArrow = []string{
		"       ▄▄      ",
		"      ████     ",
		"     ██████    ",
		"       ██      ",
	}
	logoAnvil = []string{
		" ▄▄▄▄▄▄▄▄▄▄▄▄▄ ",
		"  ▀▀▀█████████ ",
		"      ▀████▀   ",
		"     ▄██████▄  ",
	}
)

const (
	orange = "\x1b[38;2;232;104;48m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	cyan   = "\x1b[36m"
	reset  = "\x1b[0m"
)

// paint colours s when color is on.
func paint(color bool, code, s string) string {
	if !color || s == "" {
		return s
	}
	return code + s + reset
}

// banner is the logo with the name and version beside it, in a rounded box. Colours are left out
// when the output isn't a terminal (a file, a pipe) or NO_COLOR is set.
func banner(ver string, color bool) string {
	p := func(code, s string) string { return paint(color, code, s) }
	text := []string{
		"",
		p(bold, "Reforge "+ver),
		"",
		p(dim, "A package manager for"),
		p(dim, "GitHub releases."),
		"",
		"",
		p(dim, "github.com/VGRichi/Reforge"),
	}
	const textWidth = 30 // keeps the box 53 characters wide, so it fits narrow windows
	logo := append(append([]string{}, logoArrow...), logoAnvil...)
	var b strings.Builder
	border := strings.Repeat("─", utf8.RuneCountInString(logo[0])+textWidth+4)
	b.WriteString(p(orange, "╭"+border+"╮") + "\n")
	for i, art := range logo {
		if i < len(logoArrow) {
			art = p(orange, art)
		}
		line := text[i]
		pad := strings.Repeat(" ", textWidth-visibleWidth(line))
		fmt.Fprintf(&b, "%s %s  %s%s %s\n", p(orange, "│"), art, line, pad, p(orange, "│"))
	}
	b.WriteString(p(orange, "╰"+border+"╯") + "\n")
	return b.String()
}

// visibleWidth counts the characters a terminal shows, skipping colour codes.
func visibleWidth(s string) int {
	n, inCode := 0, false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inCode = true
		case inCode:
			inCode = r != 'm'
		default:
			n++
		}
	}
	return n
}

const doubleClickHelp = `
Reforge is a command-line tool, so it runs in a terminal rather than in a window.
Open PowerShell, Command Prompt or Windows Terminal and type, for example:

  reforge add                 find apps you already have, and pick what to track
  reforge list                what you track, and which have updates
  reforge --help              all commands

Press Enter to close.`
