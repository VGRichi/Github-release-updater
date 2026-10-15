# Reforge in Go: design note

Reforge 1.1 is a rewrite of the PowerShell module (GitHubAppUpdater 1–2, Reforge 3 unreleased) as a
single program, `reforge`, so it can run natively on Windows, macOS and Linux. Windows comes first, with
the same behaviour as the module; macOS and Linux follow.

It starts at 1.1.0, not 1.0.0: Go's module mirror (proxy.golang.org) permanently keeps the old v1.0.0
and v2.0.0 tags (the PowerShell versions), so reusing v1.0.0 would make `go install ...@latest` fetch
the old script. Never reuse a deleted version tag.

## Commands

```
reforge add [name]           track an app, mod or tool (installing it first if needed);
                             with no name, find what's on this PC and pick what to track
reforge list [name...]       list what's tracked: installed and latest versions, folders
reforge update [name...]     install the updates
reforge remove <name...>     stop tracking
```

Few commands that each do the whole job: `add` finds the repository, works out the kind of app, and
installs it if needed, so there's no separate install or find command.

Flags mirror the module's parameters (`--name`, `--dest`, `--subfolder`, `--include`, `--keep`,
`--rename from=to`, `--pattern`, `--silent-args`, `--prerelease`, `--allow-unverified`, `--force`,
`--yes`). `--json` gives machine-readable output, `--config` another list file. The GitHub token comes
from `GITHUB_TOKEN` or `GH_TOKEN`, or else the GitHub CLI's sign-in (`gh auth token`), never a flag, so
it doesn't end up in shell history.

## Kept from the module

- **The list file**: same JSON fields, in the same place on Windows (`%APPDATA%\Reforge\apps.json`;
  `os.UserConfigDir()` gives `~/Library/Application Support` and `~/.config` elsewhere). It also reads
  lists written by Windows PowerShell (byte-order mark, a single string where a list is expected).
- **The rules**: release file scoring (architecture, msi/exe, platform words), checksum policy
  (verified / ask / trusted / always ask if earlier releases had checksums / "No" when nobody can be
  asked), statuses, all-or-nothing archive copies with backups, finding apps by links, downloads
  (Mark of the Web) and the winget skip.

## Added in Go

- **Size check**: a download that isn't the size GitHub lists is broken and deleted.
- **Signatures** (Windows, Authenticode): a broken signature installs nothing. The signer is remembered
  per app (`Publisher` in the list); a release signed by someone else, or no longer signed, warns and
  asks (default No, never skipped by `--yes` or trusting the app), since a hacked account can publish a
  release with a valid checksum but not with the developer's signature.
- **`reforge list`** checks GitHub (installed and latest version per app), replacing `update --dry-run`.
- **Downloads** (`Type: download`, `Folder`): a release of archives (DXVK, OptiScaler) kept up to date
  where you keep it. New archives go next to the old ones; if the old version was extracted (a folder
  named like `dxvk-3.1.1`), the new one is too and the old folder moves to the backups.
- **`reforge add` works out the kind**: installed + installer in the release = installer app; a release
  of archives = download, found in Downloads (two levels deep) with the version read from names; a
  folder argument = download there; `--dest` = copied into folders.
- **`reforge add <name>`**: the repository comes from this PC (an installed app linking to GitHub, a
  download's note), else from GitHub's search, ranked exact name first, then stars, archived last;
  each match with releases is asked about (Yes/No, up to 5; nobody to ask = No).
- **Fewer API requests** (60 an hour without a token, per internet connection): answers about latest
  releases are reused for an hour (`cache.json` next to the list), a release
  published less than an hour before it was checked only for 5 minutes (its files may still be
  uploading), and "no stable release" is remembered for a day, saving a request per check. Rate-limit
  errors say when the limit resets and how to sign in. Only the official API is used: ETag requests
  still counted without a token (tested), and the web's /releases/latest redirect isn't an API.
- **Double-click** (Windows): shows the help and turns the window into a PowerShell prompt, instead of
  a window that closes at once.

## Layout

| Package | Job |
|---|---|
| `cmd/reforge` | flags, output, prompts |
| `internal/core` | add, update and find: the rules, independent of the OS |
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
