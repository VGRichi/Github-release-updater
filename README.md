<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/reforge-logo-dark.svg">
    <img src="assets/reforge-logo.svg" alt="Reforge" width="480">
  </picture>
</p>

**Keeps Windows apps and game mods from GitHub releases up to date**, the way winget does for everything else.

- **Updates installed apps** (`.msi` / `.exe`) silently, with the right installer for your PC (x64 or ARM64)
- **Installs new apps** straight from a GitHub repository
- **Updates game mods and tools** like DXVK and OptiScaler (`.zip` / `.7z` / `.tar.gz`) in your game folders, with backups, and without touching your settings
- **Finds what you already have**: installed apps that link to GitHub, and GitHub downloads in your Downloads folder
- **Verifies every download** against GitHub's SHA256 checksums, and never installs an unverified file without asking

## Install

```powershell
Install-Module Reforge -Scope CurrentUser
```

Or extract `Reforge.zip` from the [latest release](https://github.com/VGRichi/Reforge/releases/latest) into your Modules folder. Needs Windows 10 or 11, with PowerShell 7 or Windows PowerShell 5.1.

## Use

```powershell
Find-ReforgeApp | Out-GridView -PassThru | Add-ReforgeApp     # pick from what you already have
Add-ReforgeApp storytold/photocraft -Name PhotoCraft           # track an installed app
Install-ReforgeApp storytold/pdfcraft                          # install a new app and track it
Add-ReforgeApp doitsujin/dxvk -Destination 'D:\Games\Witcher 3\bin\x64' -Subfolder x64   # a game mod

Update-ReforgeApp -WhatIf                                      # see what has updates
Update-ReforgeApp                                              # update everything
```

`-Name` is the app's name in **Settings → Apps → Installed apps**. `Get-ReforgeApp` lists what you track, and `Remove-ReforgeApp` stops tracking. Apps installed for all users need PowerShell running as administrator.

| Option | Purpose |
|---|---|
| `-Subfolder` `-Include` | Copy one folder from the archive, or only matching files (`'*.dll'`) |
| `-Rename` | Rename files while copying (`@{ 'OptiScaler.dll' = 'dxgi.dll' }`) |
| `-Keep` | Never overwrite these files, like your settings (`'*.ini'`) |
| `-Pattern` | Pick the release file yourself (`'*win-x64.msi'`) |
| `-SilentArgs` | Silent-install switches for an installer that isn't recognised |
| `-Prerelease` | Follow beta releases |
| `-AllowUnverified` | Trust an app whose releases have no checksum |
| `-Token` | More than GitHub's 60 checks an hour (or set `GITHUB_TOKEN`) |

Every command has full help: `Get-Help Update-ReforgeApp -Examples`.

## Security

- **With a GitHub checksum:** verified, then installed. If it doesn't match, nothing is installed.
- **Without one:** you're asked: **Yes**, **Always** (trust this app), or **No**. If the app's earlier releases had checksums, it always asks.
- **Nobody to ask** (a scheduled task): the answer is No.

A checksum proves you got exactly the file on the release page, not that the release is trustworthy. Only track apps from developers you trust. DLL mods can trigger anti-cheat in online games.

## Limitations

- Versions are compared by number, so `1.2.0-beta3` → `1.2.0-beta4` isn't seen as an update.
- Game mods and tools aren't tracked by Windows, so their first update installs the latest release.

## License

[MIT](LICENSE). Thanks to the Reddit commenters whose feedback shaped the security rules. Working on Reforge? See [CONTRIBUTING.md](CONTRIBUTING.md).
