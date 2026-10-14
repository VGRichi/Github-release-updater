package main

import (
	"strings"
	"testing"
)

func TestBannerLinesLineUp(t *testing.T) {
	for _, color := range []bool{false, true} {
		lines := strings.Split(strings.TrimSuffix(banner("1.1.1", color), "\n"), "\n")
		if len(lines) != len(logoArrow)+len(logoAnvil)+2 {
			t.Fatalf("color=%v: %d lines", color, len(lines))
		}
		for _, line := range lines {
			if w := visibleWidth(line); w != visibleWidth(lines[0]) {
				t.Errorf("color=%v: line is %d wide, the top is %d: %q", color, w, visibleWidth(lines[0]), line)
			}
		}
	}
}

func TestBannerColorsOnlyWhenAsked(t *testing.T) {
	plain := banner("1.1.1", false)
	if strings.Contains(plain, "\x1b[") || !strings.Contains(plain, "Reforge 1.1.1") {
		t.Errorf("plain banner: %q", plain)
	}
	if colored := banner("1.1.1", true); !strings.Contains(colored, orange) || !strings.Contains(colored, reset) {
		t.Error("the colored banner has no colours")
	}
}

func TestVisibleWidthSkipsColorCodes(t *testing.T) {
	if w := visibleWidth(orange + "│ab" + reset); w != 3 {
		t.Errorf("got %d", w)
	}
}
