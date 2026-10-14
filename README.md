<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/reforge-logo-dark.svg">
    <img src="assets/reforge-logo.svg" alt="Reforge" width="480">
  </picture>
</p>

**Keeps apps and game mods from GitHub releases up to date**, the way winget does for everything else. One small program, nothing else to install.

- **Updates installed apps** (`.msi` / `.exe`) silently, with the right installer for your PC (x64 or ARM64)
- **Installs new apps** straight from a GitHub repository
- **Updates game mods and tools** like DXVK and OptiScaler (`.zip` / `.7z` / `.tar.gz`) in your game folders, with backups, and without touching your settings
- **Finds what you already have**: installed apps that link to GitHub, and GitHub downloads in your Downloads folder
- **Verifies every download** against GitHub's SHA256 checksums, and never installs an unverified file without asking

Installer apps are Windows-only for now; game mods and tools work on Windows, macOS and Linux.

## Install

**Windows** (PowerShell):

```powershell
$dir = "$env:LOCALAPPDATA\Programs\Reforge"
Invoke-WebRequest https://github.com/VGRichi/Reforge/releases/latest/download/reforge_windows_amd64.zip -OutFile "$env:TEMP\reforge.zip"
Expand-Archive "$env:TEMP\reforge.zip" $dir -Force
[Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ";$dir", 'User')
```

**macOS / Linux**: `curl -L https://github.com/VGRichi/Reforge/releases/latest/download/reforge_linux_amd64.tar.gz | tar -xz -C ~/.local/bin reforge` (use `darwin` for macOS and `arm64` for ARM).

Or download a file from the [latest release](https://github.com/VGRichi/Reforge/releases/latest), or `go install github.com/VGRichi/Reforge/cmd/reforge@latest`.

## Use

```
reforge find --add                                         pick from what you already have
reforge add storytold/photocraft --name PhotoCraft         track an installed app
reforge install storytold/pdfcraft                         install a new app and track it
reforge add doitsujin/dxvk --dest "D:\Games\Witcher 3\bin\x64" --subfolder x64   a game mod

reforge update --dry-run                                   see what has updates
reforge update                                             update everything
```

`--name` is the app's name in **Settings → Apps → Installed apps**. `reforge list` shows what you track, and `reforge remove` stops tracking. Apps installed for all users need an administrator terminal.

| Option | Purpose |
|---|---|
| `--subfolder` `--include` | Copy one folder from the archive, or only matching files (`'*.dll'`) |
| `--rename` | Rename a file while copying (`OptiScaler.dll=dxgi.dll`) |
| `--keep` | Never overwrite these files, like your settings (`'*.ini'`) |
| `--pattern` | Pick the release file yourself (`'*win-x64.msi'`) |
| `--silent-args` | Silent-install switches for an installer that isn't recognised |
| `--prerelease` | Follow beta releases |
| `--allow-unverified` | Trust an app whose releases have no checksum |
| `--json` | Machine-readable output, for scripts |

Every command has help: `reforge update --help`. For more than GitHub's 60 checks an hour, set `GITHUB_TOKEN` to a [GitHub token](https://github.com/settings/tokens) (no permissions needed).

## Security

- **With a GitHub checksum:** verified, then installed. If it doesn't match, nothing is installed.
- **Without one:** you're asked: **Yes**, **Always** (trust this app), or **No**. If the app's earlier releases had checksums, it always asks.
- **Nobody to ask** (a scheduled task): the answer is No.

A checksum proves you got exactly the file on the release page, not that the release is trustworthy. Only track apps from developers you trust. DLL mods can trigger anti-cheat in online games.

## Limitations

- Versions are compared by number, so `1.2.0-beta3` → `1.2.0-beta4` isn't seen as an update.
- Game mods and tools aren't tracked by the OS, so their first update installs the latest release.

## License

[MIT](LICENSE). Thanks to the Reddit commenters whose feedback shaped the security rules. Working on Reforge? See [CONTRIBUTING.md](CONTRIBUTING.md).
