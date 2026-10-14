// Package wildcard matches names against wildcards like '*.dll', ignoring case.
package wildcard

import "strings"

// Match reports whether name matches pattern, where * matches any run of characters (including path
// separators), ? one character, and [abc] or [a-z] one of a set.
func Match(pattern, name string) bool {
	return match([]rune(strings.ToLower(pattern)), []rune(strings.ToLower(name)))
}

// MatchPath is Match with / and \ treated as the same character, so 'bin\*' matches "bin/x.dll".
func MatchPath(pattern, path string) bool {
	return Match(strings.ReplaceAll(pattern, `\`, "/"), strings.ReplaceAll(path, `\`, "/"))
}

func match(p, s []rune) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			if len(p) == 0 {
				return true
			}
			for i := range len(s) + 1 {
				if match(p, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
		case '[':
			end := indexRune(p, ']')
			if end > 1 { // a set; '[' without a closing ']' is a plain character
				if len(s) == 0 || !inSet(p[1:end], s[0]) {
					return false
				}
				p, s = p[end+1:], s[1:]
				continue
			}
			fallthrough
		default:
			if len(s) == 0 || s[0] != p[0] {
				return false
			}
		}
		p, s = p[1:], s[1:]
	}
	return len(s) == 0
}

func inSet(set []rune, c rune) bool {
	for i := 0; i < len(set); i++ {
		if i+2 < len(set) && set[i+1] == '-' {
			if c >= set[i] && c <= set[i+2] {
				return true
			}
			i += 2
		} else if set[i] == c {
			return true
		}
	}
	return false
}

func indexRune(s []rune, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}
