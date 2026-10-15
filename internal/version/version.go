// Package version reads version numbers out of tags and installed-app versions.
package version

import (
	"regexp"
	"strconv"
	"strings"
)

// Version is up to four numbers, like 1.2.3.4. Missing parts count as 0, so 1.2 equals 1.2.0.0.
type Version []int

var number = regexp.MustCompile(`\d+(?:\.\d+){0,3}`)

// Text is the first version number in text, as written: "3.1.1" in "dxvk-3.1.1", or "".
func Text(text string) string { return number.FindString(text) }

// Parse takes the first version number in text: "v1.2.3" is 1.2.3 and "release-2024.10" is 2024.10.
// Anything after the numbers is ignored, so "1.2.0-beta3" is 1.2.0. It returns nil without a number.
func Parse(text string) Version {
	m := Text(text)
	if m == "" {
		return nil
	}
	var v Version
	for _, part := range strings.Split(m, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		v = append(v, n)
	}
	return v
}

// Compare returns -1, 0 or 1 as a is older than, the same as, or newer than b.
func Compare(a, b Version) int {
	for i := range 4 {
		x, y := part(a, i), part(b, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Same reports whether two texts hold the same version, however they're written ("v1.2.0", "1.2.0.0").
func Same(a, b string) bool {
	va, vb := Parse(a), Parse(b)
	return va != nil && vb != nil && Compare(va, vb) == 0
}

func part(v Version, i int) int {
	if i < len(v) {
		return v[i]
	}
	return 0
}
