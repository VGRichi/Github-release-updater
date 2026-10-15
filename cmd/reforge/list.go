package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/VGRichi/Reforge/internal/applist"
	"github.com/VGRichi/Reforge/internal/core"
)

// listEntry is one app in 'reforge list': what's tracked, and what checking its latest release found.
type listEntry struct {
	Name        string   `json:"name"`
	Repository  string   `json:"repository"`
	Type        string   `json:"type"`
	Installed   string   `json:"installed"`
	Latest      string   `json:"latest"`
	Status      string   `json:"status"`
	Note        string   `json:"note"`
	Destination []string `json:"destination,omitempty"`
	Folder      string   `json:"folder,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Trusted     bool     `json:"trusted"`
}

// runList shows what's tracked and checks each app's latest release (installing nothing).
func runList(c *cli, args []string) error {
	fs := c.flags("list")
	asJSON := fs.Bool("json", false, "print the list as JSON")
	names, err := parse(fs, args)
	if err != nil {
		return err
	}
	apps, listPath, err := c.load()
	if err != nil {
		return err
	}
	if len(apps) == 0 && !*asJSON {
		fmt.Fprintln(c.stderr, "No apps tracked yet. Add one by name with 'reforge add <name>', or pick from what's on this PC with 'reforge add'.")
		return nil
	}
	e := c.engine(listPath)
	e.DryRun = true // a check: nothing is downloaded, and list never saves the list
	entries := []listEntry{}
	var rows [][]string
	for _, a := range c.matching(apps, names) {
		r := e.Update(a)
		le := listEntry{Name: a.Name, Repository: a.Repository, Type: "Installer", Installed: r.Installed, Latest: r.Latest,
			Status: string(r.Status), Note: r.Note, Destination: a.Destination, Publisher: a.Publisher, Trusted: a.AllowUnverified}
		folder := strings.Join(a.Destination, "; ")
		switch {
		case a.IsArchive():
			le.Type = "Archive"
		case a.IsDownload():
			le.Type, le.Folder, folder = "Download", a.Folder, a.Folder
		}
		c.failed = c.failed || r.Status == core.Failed
		entries = append(entries, le)
		rows = append(rows, []string{le.Name, le.Installed, le.Latest, listStatus(r), folder})
	}
	if *asJSON {
		return c.json(entries)
	}
	c.table([]string{"Name", "Installed", "Latest", "Status", "Folder"}, rows)
	return nil
}

func listStatus(r core.Result) string {
	switch r.Status {
	case core.UpdateAvailable:
		if strings.HasPrefix(r.Note, "No checksum") {
			return readable(r.Status) + " (no checksum)"
		}
		return readable(r.Status)
	case core.UpToDate, core.NotInstalled:
		return readable(r.Status)
	case core.Failed:
		return "Error: " + r.Note
	}
	return r.Note // skipped, like "Folder not found"
}

// runUpdate installs the updates, saving what each app's update changed in the list.
func runUpdate(c *cli, args []string) error {
	fs := c.flags("update")
	asJSON := fs.Bool("json", false, "print the results as JSON")
	fs.BoolVar(&c.verbose, "verbose", false, "show more detail")
	names, err := parse(fs, args)
	if err != nil {
		return err
	}
	apps, listPath, err := c.load()
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		c.warn("no apps in the list yet. Add one by name with 'reforge add <name>', or pick from what's on this PC with 'reforge add'.")
		return nil
	}
	e := c.engine(listPath)
	e.CleanWorkRoot()
	var results []core.Result
	for _, app := range c.matching(apps, names) {
		before, _ := json.Marshal(app)
		e.Progress = func(msg string) { c.progress(app.Name + ": " + msg) }
		r := e.Update(app)
		if after, _ := json.Marshal(app); !bytes.Equal(before, after) {
			if err := applist.Save(listPath, apps); err != nil {
				return err
			}
		}
		c.failed = c.failed || r.Status == core.Failed
		results = append(results, r)
	}
	return c.printResults(results, *asJSON)
}

// runRemove stops tracking apps; they stay installed.
func runRemove(c *cli, args []string) error {
	fs := c.flags("remove")
	names, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fs.Usage()
		return errUsage
	}
	apps, listPath, err := c.load()
	if err != nil {
		return err
	}
	removed := c.matching(apps, names)
	if len(removed) == 0 {
		return nil
	}
	var keep []*applist.App
	for _, a := range apps {
		if matchesAny(a.Name, names) {
			fmt.Fprintf(c.stdout, "Stopped tracking %s\n", a.Name)
		} else {
			keep = append(keep, a)
		}
	}
	return applist.Save(listPath, keep)
}

func (c *cli) printResults(results []core.Result, asJSON bool) error {
	if asJSON {
		if results == nil {
			results = []core.Result{}
		}
		return c.json(results)
	}
	if len(results) == 0 {
		return nil
	}
	var rows [][]string
	for _, r := range results {
		rows = append(rows, []string{r.Name, r.Installed, r.Latest, readable(r.Status), r.Note})
	}
	c.table([]string{"Name", "Installed", "Latest", "Status", "Note"}, rows)
	return nil
}

// readable spells a status out for people ("Up to date"); --json keeps the exact values for scripts.
func readable(s core.Status) string {
	switch s {
	case core.UpToDate:
		return "Up to date"
	case core.UpdateAvailable:
		return "Update available"
	case core.NotInstalled:
		return "Not installed"
	case core.NeedsAdmin:
		return "Needs admin"
	}
	return string(s)
}
