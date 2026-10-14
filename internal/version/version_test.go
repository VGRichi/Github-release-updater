package version

import "testing"

func TestParse(t *testing.T) {
	tests := []struct{ text, want string }{
		{"v1.2.3", "1.2.3"},
		{"1.2.3.4", "1.2.3.4"},
		{"2", "2"},
		{"release-2024.10", "2024.10"},
		{"1.2.3.4.5", "1.2.3.4"},
		{"1.2.0-beta3", "1.2.0"},
		{"App2-v1.3", "2"}, // the first number, even inside a name
		{"v2.56.0.windows.2", "2.56.0"},
	}
	for _, tt := range tests {
		if got := Parse(tt.text).String(); got != tt.want {
			t.Errorf("Parse(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
	for _, text := range []string{"nightly", ""} {
		if got := Parse(text); got != nil {
			t.Errorf("Parse(%q) = %v, want nil", text, got)
		}
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.10.0", "v1.9.0", 1}, // numbers, not text
		{"1.2", "1.2.0.0", 0},
		{"7.6.6.0", "v7.6.6", 0},
		{"1.0.0", "1.0.1", -1},
	}
	for _, tt := range tests {
		if got := Compare(Parse(tt.a), Parse(tt.b)); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSame(t *testing.T) {
	if !Same("v8.9.8.1", "8.9.8.1") || !Same("v0.5.0", "0.5") {
		t.Error("versions written differently should be the same")
	}
	if Same("v0.4.0", "0.5.0") || Same("nightly", "nightly") {
		t.Error("different or missing versions should not be the same")
	}
}
