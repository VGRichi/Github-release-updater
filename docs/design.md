# Reforge in Go: design note

Reforge 1.1 is a rewrite of the PowerShell module (GitHubAppUpdater 1–2, Reforge 3 unreleased) as a
single program, `reforge`, so it can run natively on Windows, macOS and Linux. Windows comes first, with
the same behaviour as the module; macOS and Linux follow.

It starts at 1.1.0, not 1.0.0: Go's module mirror (proxy.golang.org) permanently keeps the old v1.0.0
and v2.0.0 tags (the PowerShell versions), so reusing v1.0.0 would make `go install ...@latest` fetch
the old script. Never reuse a deleted version tag.

## Commands

```
reforge add <owner/repo>       start tracking an app or tool
reforge install <owner/repo>   install an app for the first time, and track it
reforge list [name...]         list what's tracked
reforge update [name...]       check for updates and install them (--dry-run to only check)
reforge find                   find installed apps and downloaded tools that come from GitHub
reforge remove <name...>       stop tracking
```

Flags mirror the module's parameters (`--name`, `--dest`, `--subfolder`, `--include`, `--keep`,
`--rename from=to`, `--pattern`, `--silent-args`, `--prerelease`, `--allow-unverified`, `--force`,
`--yes`). `--json` gives machine-readable output, `--config` another list file. The GitHub token comes
from `GITHUB_TOKEN` or `GH_TOKEN` only, never a flag, so it doesn't end up in shell history.

## Kept from the module

- **The list file**: same JSON fields, in the same place on Windows (`%APPDATA%\Reforge\apps.json`;
  `os.UserConfigDir()` gives `~/Library/Application Support` and `~/.config` elsewhere). It also reads
  lists written by Windows PowerShell (byte-order mark, a single string where a list is expected).
- **The rules**: release file scoring (architecture, msi/exe, platform words), checksum policy
  (verified / ask / trusted / always ask if earlier releases had checksums / "No" when nobody can be
  asked), statuses, all-or-nothing archive copies with backups, finding apps by links, downloads
  (Mark of the Web) and the winget skip.

## Layout

| Package | Job |
|---|---|
| `cmd/reforge` | flags, output, prompts |
| `internal/core` | update, install and find: the rules, independent of the OS |
| `internal/system` | the OS: installed apps, running installers, admin check, download notes, winget |
| `internal/github` | releases API, downloads with checksum checks |
| `internal/pick` | choosing the release file |
| `internal/archive` | extracting, copying with backups and rollback |
| `internal/applist`, `internal/version`, `internal/wildcard` | the list file, version numbers, `*` patterns |

`internal/system` is the only OS-specific package: `system_windows.go` (registry, ShellExecuteEx so
installers can ask for admin, `:Zone.Identifier`, `winget list`) and `system_other.go`. `core` takes
it as an interface, so every rule is tested with a fake system and a fake GitHub on all three OSes.

## Dependencies

Standard library plus `golang.org/x/sys` (registry, process tokens). `.zip` and `.tar.gz` are
extracted in Go; `.7z` and `.tar.xz` use 7-Zip or the system `tar`, as the module did.

## Later

- **macOS / Linux**: installed apps (`/Applications`, `dpkg`/`rpm`), installers (`.pkg`/`.dmg`,
  `.deb`/`.rpm`, AppImage), download notes (`kMDItemWhereFroms`, `user.xdg.origin.url`). Archive tools
  already work everywhere.
- A single-file tool type (one program, like `yt-dlp.exe`), other forges (GitLab, Codeberg/Forgejo,
  Gitea), and publishing to winget, Scoop and Homebrew.
