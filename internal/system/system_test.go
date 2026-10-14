package system

import (
	"reflect"
	"testing"
)

func TestParseWingetList(t *testing.T) {
	// Real `winget list --source winget` output, with a progress spinner before the table
	out := "   - \r   \\ \r" +
		"Name                     Id                          Version  Available Source\r\n" +
		"------------------------------------------------------------------------------\r\n" +
		"Notepad++                Notepad++.Notepad++         8.9.8.1           winget\r\n" +
		"PowerShell               Microsoft.PowerShell        7.6.6.0  7.6.7    winget\r\n" +
		"Microsoft Visual C++ 20… Microsoft.VCRedist.2015+.x64 14.44.0          winget\r\n"
	want := []Package{
		{"Notepad++", "8.9.8.1"},
		{"PowerShell", "7.6.6.0"},
		{"Microsoft Visual C++ 20", "14.44.0"},
	}
	if got := parseWingetList(out); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestParseWingetListTranslatedHeaders(t *testing.T) {
	// Only column positions are used, so German headers work the same
	out := "Name           ID                          Version  Verfügbar Quelle\n" +
		"--------------------------------------------------------------------\n" +
		"BCUninstaller  Klocman.BulkCrapUninstaller 6.3.0.0           winget\n"
	if got := parseWingetList(out); !reflect.DeepEqual(got, []Package{{"BCUninstaller", "6.3.0.0"}}) {
		t.Errorf("got %v", got)
	}
}

func TestParseWingetListWithoutTable(t *testing.T) {
	if got := parseWingetList("No installed package found matching input criteria.\n"); got != nil {
		t.Errorf("got %v", got)
	}
}
