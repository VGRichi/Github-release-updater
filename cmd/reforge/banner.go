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
	reset  = "\x1b[0m"
)

// banner is the logo with the name and version beside it, in a rounded box. Colours are left out
// when the output isn't a terminal (a file, a pipe) or NO_COLOR is set.
func banner(ver string, color bool) string {
	paint := func(code, s string) string {
		if !color || s == "" {
			return s
		}
		return code + s + reset
	}
	text := []string{
		"",
		paint(bold, "Reforge "+ver),
		"",
		paint(dim, "Download, install and update"),
		paint(dim, "apps and game mods from"),
		paint(dim, "GitHub releases."),
		"",
		paint(dim, "github.com/VGRichi/Reforge"),
	}
	const textWidth = 30 // keeps the box 53 characters wide, so it fits narrow windows
	logo := append(append([]string{}, logoArrow...), logoAnvil...)
	var b strings.Builder
	border := strings.Repeat("─", utf8.RuneCountInString(logo[0])+textWidth+4)
	b.WriteString(paint(orange, "╭"+border+"╮") + "\n")
	for i, art := range logo {
		if i < len(logoArrow) {
			art = paint(orange, art)
		}
		line := text[i]
		pad := strings.Repeat(" ", textWidth-visibleWidth(line))
		fmt.Fprintf(&b, "%s %s  %s%s %s\n", paint(orange, "│"), art, line, pad, paint(orange, "│"))
	}
	b.WriteString(paint(orange, "╰"+border+"╯") + "\n")
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

  reforge find --add          track apps you already have
  reforge list                what you track, and which have updates
  reforge --help              all commands

Press Enter to close.`
