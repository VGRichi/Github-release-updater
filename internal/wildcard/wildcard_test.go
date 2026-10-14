package wildcard

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"*.dll", "d3d11.dll", true},
		{"*.DLL", "D3D11.dll", true},
		{"*.dll", "d3d11.dll.bak", false},
		{"*win-x64.msi", "App-1.0-win-x64.msi", true},
		{"App-?.zip", "App-1.zip", true},
		{"App-?.zip", "App-12.zip", false},
		{"App-[0-9].zip", "App-7.zip", true},
		{"App-[0-9].zip", "App-x.zip", false},
		{"App-[xy].zip", "App-y.zip", true},
		{"[notaset.zip", "[notaset.zip", true},
		{"*", "", true},
		{"", "", true},
		{"", "a", false},
		{"*setup*", "PhotoCraft-Setup-x64.exe", true},
	}
	for _, tt := range tests {
		if got := Match(tt.pattern, tt.name); got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{`bin\*`, "bin/x.dll", true},
		{`bin/*`, `bin\x.dll`, true},
		{`bin\*`, `bin\sub\x.dll`, true}, // * crosses folders, like PowerShell's -like
		{`config\*`, "bin/x.dll", false},
	}
	for _, tt := range tests {
		if got := MatchPath(tt.pattern, tt.path); got != tt.want {
			t.Errorf("MatchPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}
