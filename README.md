# GitHub App Updater

Keep Windows apps you installed from **GitHub releases** up to date, the way `winget upgrade` does for everything else.

Many great apps are only published on GitHub, so winget can't update them. This script remembers which repo each app came from, checks for new releases, and installs them silently.

## Features

- Compares your installed version with the latest GitHub release
- Picks the right installer for your PC (x64 or ARM64, and MSI or EXE to match what you already have)
- Verifies the download's SHA256 hash when GitHub provides one
- Installs silently: MSI, Inno Setup and NSIS installers are detected automatically
- `-Discover` finds GitHub apps you already have by reading their uninstall info
- Skips apps that winget already updates, so nothing is updated twice
- Asks for admin only once per run

## Install

Paste this into PowerShell:

```powershell
irm https://raw.githubusercontent.com/VGRichi/github-app-updater/main/install.ps1 | iex
```

This downloads the script into your PowerShell `Scripts` folder and adds an `upgh` command to your profile. Open a new PowerShell window afterwards.

> It's always good practice to read a script before running it with `iex`: [install.ps1](install.ps1)

**Manual install:** download `Update-GitHubApps.ps1`, run `Unblock-File .\Update-GitHubApps.ps1`, and run it directly.

## Usage

```powershell
upgh -Discover                              # find GitHub apps you have installed, then update
upgh -Add storytold/photocraft -Name PhotoCraft   # add an app to the list
upgh -List                                  # show the list
upgh -CheckOnly                             # show what's outdated, install nothing
upgh                                        # update everything
```

`-Name` is the app's name as shown in **Settings → Apps → Installed apps** (partial names work). `-Add` also accepts full links like `https://github.com/storytold/photocraft`.

## Using it from VS Code

Clone or open this repo in VS Code, then press **Ctrl + Shift + P → Tasks: Run Task** and choose:

- **GitHub apps: check for updates**
- **GitHub apps: update all**
- **GitHub apps: discover and update**
- **GitHub apps: show list**

Updating opens a separate admin window with one UAC prompt.

## The app list

Your list is stored in `%APPDATA%\GitHubAppUpdater\github-apps.json`. Besides `Name` and `Repo`, each app can have:

| Option | When to use it |
|---|---|
| `"Pattern": "*win-x64-setup.exe"` | The script picks the wrong file from the release |
| `"SilentArgs": "--silent"` | The installer type isn't detected automatically |
| `"Prerelease": true` | You want to follow beta releases |

See [github-apps.example.json](github-apps.example.json).

## Limitations

- **Version numbers only:** `1.2.0-beta3` and `1.2.0-beta4` both count as `1.2.0`, so beta-to-beta updates of the same version are not detected.
- **Apps that report the wrong version** (e.g. the installer says 5.0.0 while the app is 6.0) will be reinstalled every run. Remove them from the list.
- **Portable `.zip` apps** are not updated, only installers (`.msi` / `.exe`).
- **GitHub rate limit:** about 60 checks per hour without logging in. To raise it, set a `GITHUB_TOKEN` environment variable with a personal access token.

## Requirements

Windows 10 or 11, PowerShell 7 (recommended) or Windows PowerShell 5.1.

## License

[MIT](LICENSE)
