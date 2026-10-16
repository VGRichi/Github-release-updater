// Command reforge is a package manager for GitHub releases: it installs and updates apps and game
// mods straight from them.
package main

// reforge.exe's icon is in the rsrc_windows_*.syso files, which go build links in. Remake them after
// changing the icon with: go generate ./cmd/reforge. Releases set REFORGE_VERSION, which also adds the
// name, version and copyright shown in File > Properties > Details.
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --manifest cli --icon ../../assets/reforge-app-icon-512.png --product-name Reforge --file-description Reforge --original-filename reforge.exe --copyright "(c) 2026 VGRichi. MIT License." --product-version "$REFORGE_VERSION" --file-version "$REFORGE_VERSION"

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/VGRichi/Reforge/internal/archive"
)

// version is set when building a release: go build -ldflags "-X main.version=1.0.0"
var version = "dev"

const usage = `Usage: reforge <command> [arguments] [options]

Commands:
  add [name]         track an app, mod or tool, installing it if it isn't installed yet;
                     with no name, find what's on this PC and pick what to track
  list [name...]     what you track, and which have updates
  update [name...]   install the updates
  remove <name...>   stop tracking (doesn't uninstall)

Run 'reforge <command> --help' for its options. Names can use wildcards like 'DXVK*'.
For more than GitHub's 60 checks an hour, sign in with the GitHub CLI (gh auth login) or set GITHUB_TOKEN.
`

var commands = map[string]func(*cli, []string) error{
	"add": runAdd, "list": runList, "update": runUpdate, "remove": runRemove,
}

// help is each command's arguments and description, for 'reforge <command> --help'.
var help = map[string][2]string{
	"add": {"[name or owner/repo] [folder]", "Starts tracking an app, mod or tool. With just a name, Reforge looks on your PC (installed\n" +
		"apps, downloads from GitHub), then searches GitHub and asks about each match. Then:\n" +
		"  - an installed app is updated with its installer;\n" +
		"  - an app that isn't installed yet is installed first (it asks, unless --yes);\n" +
		"  - a downloaded mod or tool (a .zip, .7z or .tar.gz) is kept up to date where you keep it. It's\n" +
		"    looked for in your Downloads folder, or give the folder. New versions are downloaded there,\n" +
		"    and extracted too if you extracted the old one.\n" +
		"With --dest, its files are copied into that folder (like a game's) on every update instead.\n" +
		"With no name, it finds the apps and downloads from GitHub on this PC and asks which to track."},
	"list":   {"[name...]", "Lists the apps and tools you track, with their installed and latest versions\nand folders. It checks GitHub for updates, but installs nothing."},
	"update": {"[name...]", "Installs the updates: your apps and tools go to their latest GitHub release.\nSee what has updates first with 'reforge list'."},
	"remove": {"<name...>", "Stops tracking apps. They stay installed."},
}

// color is whether the help screen's banner is in colour.
var color bool

func main() {
	color = terminalColors(os.Stdout)
	if len(os.Args) == 1 && startedByDoubleClick() {
		// Opened from Explorer: without this, the window would show the help and close at once.
		// Show the help, then keep the window as a PowerShell prompt to type commands in.
		fmt.Print(banner(version, color), "\n", usage, "\nType reforge commands below. Close the window or type 'exit' when you're done.\n\n")
		if runShell() {
			return
		}
		fmt.Print(doubleClickHelp)
		bufio.NewReader(os.Stdin).ReadString('\n')
		return
	}
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, banner(version, color), "\n", usage)
		return 0
	}
	if args[0] == "--version" { // for scripts; people see the version in the banner
		fmt.Fprintf(stdout, "reforge %s\n", version)
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "reforge: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	c := newCLI(stdin, stdout, stderr)
	err := cmd(c, args[1:])
	if c.cache != nil {
		c.cache.Save()
	}
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		return 2
	case err != nil:
		fmt.Fprintf(stderr, "%s %s\n", paint(c.colorErr, red, "reforge:"), err)
		return 1
	case c.failed:
		return 1
	}
	return 0
}

var errUsage = errors.New("usage")

// flags makes a flag set for a command, with the options every command has.
func (c *cli) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	fs.Usage = func() {
		fmt.Fprintf(c.stderr, "Usage: reforge %s %s [options]\n\n%s\n\nOptions:\n", name, help[name][0], help[name][1])
		fs.VisitAll(func(f *flag.Flag) {
			value := " <value>"
			if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
				value = ""
			}
			fmt.Fprintf(c.stderr, "  --%s%s\n        %s\n", f.Name, value, f.Usage)
		})
	}
	fs.StringVar(&c.configPath, "config", "", "use this list file instead of the default one")
	return fs
}

// parse reads flags and arguments in any order ("add owner/repo --name X" and "add --name X owner/repo").
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, errUsage
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// multi is a flag that can be given more than once, like --dest A --dest B.
type multi []string

func (m *multi) String() string     { return strings.Join(*m, ", ") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// renames is --rename from=to, more than once.
type renames map[string]string

func (r renames) String() string { return "" }
func (r renames) Set(v string) error {
	from, to, ok := strings.Cut(v, "=")
	if !ok || !archive.ValidName(from) || !archive.ValidName(to) {
		return errors.New("use --rename old.dll=new.dll (file names, not paths)")
	}
	r[from] = to
	return nil
}
