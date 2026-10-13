# GitHub App Updater

Keep Windows apps and tools you installed from **GitHub releases** up to date, the way `winget upgrade` does for everything else.

Many great apps are only published on GitHub, so winget can't update them. This script remembers which repo each app came from, checks for new releases, and installs them silently.

## Features

- Compares your installed version with the latest GitHub release
- Picks the right installer for your PC (x64 or ARM64, and MSI or EXE to match what you already have)
- Verifies downloads against GitHub's SHA256 checksum, and **never installs an unverified file silently** (see [Security](#security))
- Installs silently: MSI, Inno Setup and NSIS installers are detected automatically
- Also updates **zip / 7z / tar.gz tools** like DXVK or OptiScaler: extracts them into the folders you choose, backs up replaced files, and keeps your settings files
- `-Discover` finds GitHub apps you already have by reading their uninstall info
- Skips apps that winget already updates, so nothing is updated twice
- Asks for admin only once per run

## Install

Paste this into PowerShell:

```powershell
irm https://github.com/VGRichi/github-app-updater/releases/latest/download/install.ps1 | iex
```

This installs the **latest release** (not unreleased changes), verifies its checksum, puts the script in your PowerShell `Scripts` folder, and adds an `upgh` command to your profile. Open a new PowerShell window afterwards.

To update the tool later, run the same command again.

> It's always good practice to read a script before running it with `iex`: [install.ps1](install.ps1)

**Manual install:** download `Update-GitHubApps.ps1` from the [latest release](https://github.com/VGRichi/github-app-updater/releases/latest), run `Unblock-File .\Update-GitHubApps.ps1`, and run it directly.

## Usage

```powershell
upgh -Discover                              # find GitHub apps you have installed, then update
upgh -Add storytold/photocraft -Name PhotoCraft   # add an app to the list
upgh -List                                  # show the list
upgh -CheckOnly                             # show what's outdated, install nothing
upgh -Remove PhotoCraft                     # stop tracking an app
upgh                                        # update everything
```

Run `Get-Help upgh -Examples` to see all commands, or type `upgh -` and press **Tab** to cycle through the options.

`-Name` is the app's name as shown in **Settings → Apps → Installed apps** (partial names work). `-Add` also accepts full links like `https://github.com/storytold/photocraft`.

## Zip / 7z / tar.gz tools (DXVK, OptiScaler, etc.)

Some projects don't have an installer. You just extract their files into a game or app folder. Tell the script where they go:

```powershell
upgh -Add doitsujin/dxvk -Name "DXVK (Witcher 3)" -Destination "D:\Games\Witcher 3\bin\x64" -Subfolder x64
```

- `-Destination` is the folder the files are copied into.
- `-Subfolder` picks a folder inside the archive (DXVK has `x64` and `x32`). A single wrapper folder like `dxvk-2.5/` is skipped automatically.
- Windows doesn't track these files, so the script remembers which release it installed. The **first run always installs the latest release**, even if you already have it.
- Files that get replaced are backed up to `%APPDATA%\GitHubAppUpdater\backups` first, so you can roll back.
- `.zip` and `.tar.gz` work out of the box. For `.7z`, install 7-Zip if extraction fails: `winget install --id 7zip.7zip`.

For more control, edit the list file (see below): `Destinations` (several folders), `Include` (only copy matching files, e.g. `["*.dll"]`), `Rename` (e.g. OptiScaler's `OptiScaler.dll` → `dxgi.dll`), and `Keep` (files like `*.ini` that are added if missing but never overwritten).

> **Online games:** DLL mods like DXVK or OptiScaler can trigger anti-cheat systems. Only use them in games where that's safe.

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
| `"AllowUnverified": true` | Install releases without a checksum without asking (same as answering **a** at the prompt) |
| `"Type": "archive"` + `"Destination"` | A zip/7z/tar.gz tool that's extracted into a folder |
| `"Destinations": [...]` | The same tool goes into several folders |
| `"Subfolder": "x64"` | Copy only one folder from inside the archive |
| `"Include": ["*.dll"]` | Copy only matching files |
| `"Rename": { "a.dll": "b.dll" }` | Rename files while copying |
| `"Keep": ["*.ini"]` | Never overwrite these files (your settings) |

See [github-apps.example.json](github-apps.example.json).

## Security

The script installs with admin rights and no installer window, so it is strict about what it installs silently:

| The downloaded file… | What happens |
|---|---|
| has a GitHub checksum | Verified, then installed silently. If it doesn't match, nothing is installed. |
| has no checksum, but this app's **earlier releases had one** | Red warning and a prompt (default: no). Trusting an app never skips this, because it's unusual. |
| has no checksum, and never had one | Prompt: **y** = install this time, **a** = always trust this app, **Enter** = skip |
| has no checksum, app is trusted | Installed, with a note that it wasn't verified |

`upgh -CheckOnly` shows which updates are unverified before you run anything.

What a checksum does and doesn't prove: it confirms you got **exactly the file on the release page**, undamaged and unswapped on the way. It can't tell whether that release itself is trustworthy, for example if a developer's GitHub account were hacked. Only track apps from developers you trust.

Thanks to the Reddit commenter who pointed out that unverified files used to be installed silently, and suggested the "earlier releases had a checksum" check.

## Limitations

- **Version numbers only:** `1.2.0-beta3` and `1.2.0-beta4` both count as `1.2.0`, so beta-to-beta updates of the same version are not detected.
- **Apps that report the wrong version** (e.g. the installer says 5.0.0 while the app is 6.0) will be reinstalled every run. Remove them from the list.
- **GitHub rate limit:** about 60 checks per hour without logging in. To raise it, set a `GITHUB_TOKEN` environment variable with a personal access token.

## Publishing a new version (maintainers)

1. Commit and push your changes to `main`.
2. On GitHub, go to **Releases → Draft a new release**, create a new tag (e.g. `v1.1.0`), add notes, and click **Publish release**.
3. A GitHub Action automatically attaches `install.ps1` and `Update-GitHubApps.ps1` to the release. Users get the new version the next time they run the install command.

## Requirements

Windows 10 or 11, PowerShell 7 (recommended) or Windows PowerShell 5.1.

## License

[MIT](LICENSE)