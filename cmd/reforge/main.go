// Command reforge keeps apps and tools from GitHub releases up to date.
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
)

// version is set when building a release: go build -ldflags "-X main.version=1.0.0"
var version = "dev"

const usage = `Usage: reforge <command> [arguments] [options]

Commands:
  add <owner/repo>       start tracking an app, or a tool with --dest
  install <owner/repo>   install an app from GitHub for the first time, and track it
  list [name...]         list what you're tracking
  update [name...]       check for updates and install them (--dry-run to only check)
  find [folder...]       find installed apps and downloaded tools that come from GitHub
  remove <name...>       stop tracking (doesn't uninstall)
  version                show Reforge's version

Run 'reforge <command> --help' for its options. Names can use wildcards like 'DXVK*'.
Set GITHUB_TOKEN to a GitHub token for more than 60 checks an hour.
`

var commands = map[string]func(*cli, []string) error{
	"add": runAdd, "install": runInstall, "list": runList, "update": runUpdate, "find": runFind, "remove": runRemove,
}

// help is each command's arguments and description, for 'reforge <command> --help'.
var help = map[string][2]string{
	"add":     {"<owner/repo>", "Starts tracking an app or tool. With --dest it's an archive tool, extracted into that folder."},
	"install": {"<owner/repo>...", "Installs an app from its latest GitHub release (asking first) and starts tracking it."},
	"list":    {"[name...]", "Lists the apps and tools you're tracking."},
	"update":  {"[name...]", "Updates your apps and tools to their latest GitHub release."},
	"find":    {"[folder...]", "Finds installed apps that link to GitHub, and apps and tools downloaded from GitHub\nreleases into your Downloads folder (or the folders given, two levels deep)."},
	"remove":  {"<name...>", "Stops tracking apps. They stay installed."},
}

// color is whether the help screen's banner is in colour.
var color bool

func main() {
	color = terminalColors(os.Stdout)
	if len(os.Args) == 1 && startedByDoubleClick() {
		// Opened from Explorer: without this, the window would show the help and close at once
		fmt.Print(banner(version, color), doubleClickHelp)
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
	if args[0] == "version" || args[0] == "--version" {
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
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		return 2
	case err != nil:
		fmt.Fprintf(stderr, "reforge: %s\n", err)
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
	if !ok || from == "" || to == "" {
		return errors.New("use --rename old.dll=new.dll")
	}
	r[from] = to
	return nil
}
