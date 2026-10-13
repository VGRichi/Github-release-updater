# GitHubAppUpdater

A PowerShell module that keeps Windows apps and tools installed from **GitHub releases** up to date, the way winget does for everything else.

Many apps and game mods are only published on GitHub, so neither `winget upgrade` nor the WinGet PowerShell module can update them. GitHubAppUpdater remembers which repository each app came from, checks for new releases, verifies the downloads, and installs them.

- **Installer apps** (`.msi` / `.exe`): compared with the version in Installed apps, updated silently, with the right installer for your PC (x64 or ARM64)
- **Archive tools** (`.zip` / `.7z` / `.tar.gz`) like DXVK or OptiScaler: extracted into the folders you choose, with backups and protection for your settings files
- **Verified downloads**: checked against GitHub's SHA256 checksums, and never installed unverified without asking ([Security](#security))

## Install

```powershell
Install-Module GitHubAppUpdater -Scope CurrentUser
```

PowerShell will ask whether to trust the PowerShell Gallery; that's normal for any module. To read the code before installing: `Save-Module GitHubAppUpdater -Path .\review`.

**Without the PowerShell Gallery:** download `GitHubAppUpdater.zip` from the [latest release](https://github.com/VGRichi/github-app-updater/releases/latest) and extract it into your Modules folder. Or in PowerShell:

```powershell
$zip = "$env:TEMP\GitHubAppUpdater.zip"
Invoke-WebRequest https://github.com/VGRichi/github-app-updater/releases/latest/download/GitHubAppUpdater.zip -OutFile $zip
Unblock-File $zip
Expand-Archive $zip -DestinationPath (Join-Path (Split-Path $PROFILE) 'Modules') -Force
```

Open a new PowerShell window and the commands are available. To update, run the same lines again.

Requires Windows 10 or 11 with PowerShell 7 or Windows PowerShell 5.1.

## Quick start

```powershell
Add-GitHubApp -Repository storytold/photocraft -Name PhotoCraft   # track an app
Update-GitHubApp -WhatIf                                          # see what has updates
Update-GitHubApp                                                  # update 
```

`-Name` is the app's name as shown in **Settings → Apps → Installed apps** (a partial name is fine).
Apps installed for all users need PowerShell running as administrator to update. Apps installed only for your account, and archive tools in folders you own, don't.

## Commands

| Command | What it does |
|---|---|
| `Add-GitHubApp` | Start tracking an app or tool |
| `Get-GitHubApp` | List what you're tracking |
| `Update-GitHubApp` | Check and install updates (`-WhatIf` to only check) |
| `Find-GitHubApp` | Find installed apps that link to GitHub |
| `Remove-GitHubApp` | Stop tracking an app (doesn't uninstall it) |

Every command has full help: `Get-Help Add-GitHubApp -Examples`, or `-Online` to open its page on GitHub. Run `Update-Help -Module GitHubAppUpdater` to get the newest help without updating the module. The help pages also live in [docs/en-US](docs/en-US).

### Adding apps

```powershell
# Named parameters
Add-GitHubApp -Repository storytold/photocraft -Name PhotoCraft

# A hashtable (splatting)
$app = @{ Repository = 'storytold/pdfcraft'; Name = 'PdfCraft' }
Add-GitHubApp @app

# A CSV file with Repository and Name columns
Import-Csv .\my-apps.csv | Add-GitHubApp

# Installed apps that link to GitHub (pick them in a window)
Find-GitHubApp | Out-GridView -PassThru | Add-GitHubApp
```

### Archive tools (DXVK, OptiScaler, etc.)

Give `-Destination`, the folder the files are extracted into:

```powershell
Add-GitHubApp -Repository doitsujin/dxvk -Name 'DXVK (Witcher 3)' `
    -Destination 'D:\Games\Witcher 3\bin\x64' -Subfolder x64

Add-GitHubApp -Repository optiscaler/OptiScaler -Name 'OptiScaler (Cyberpunk)' `
    -Destination 'D:\Games\Cyberpunk 2077\bin\x64' -Rename @{ 'OptiScaler.dll' = 'dxgi.dll' } -Keep '*.ini'
```

| Option | Purpose |
|---|---|
| `-Destination` | Folder(s) the files go into; several folders are allowed |
| `-Subfolder` | Copy one folder from inside the archive (DXVK has `x64` and `x32`) |
| `-Include` | Only copy matching files, e.g. `'*.dll'` |
| `-Rename` | Rename files while copying |
| `-Keep` | Files that are added if missing but never overwritten (your settings) |

Windows doesn't track these files, so the first update always installs the latest release. Replaced files are backed up to `%APPDATA%\GitHubAppUpdater\backups`. For `.7z` archives, install 7-Zip if extraction fails: `winget install --id 7zip.7zip`.

> **Online games:** DLL mods like DXVK or OptiScaler can trigger anti-cheat systems. Only use them where that's safe.

### Other options

| Option | Purpose |
|---|---|
| `-Pattern '*win-x64.msi'` | Pick the release file yourself if the automatic choice is wrong |
| `-SilentArgs '--silent'` | Silent-install arguments for an installer type that isn't detected |
| `-Prerelease` | Follow beta releases |
| `-AllowUnverified` | Trust an app whose releases have no checksum ([Security](#security)) |
| `-Force` | Replace an app already in the list |

### GitHub rate limit

GitHub allows about 60 checks per hour without signing in. For more, pass a [personal access token](https://github.com/settings/tokens) (no permissions needed):

```powershell
Update-GitHubApp -Token (Read-Host -AsSecureString 'GitHub token')
```

or set it once in the `GITHUB_TOKEN` environment variable.

## Security

Installer apps install with admin rights and no installer window, so the module is strict about what it installs without asking:

| The downloaded file… | What happens |
|---|---|
| has a GitHub checksum | Verified, then installed. If it doesn't match, nothing is installed. |
| has no checksum, but this app's **earlier releases had one** | Warning and a prompt (default: no). Trusting an app never skips this, because it's unusual. |
| has no checksum, and never had one | Prompt: **Yes** = this time, **Always** = trust this app, **No** = skip |
| has no checksum, app is trusted | Installed, noted as unverified in the results |

`Update-GitHubApp -WhatIf` shows which updates are unverified before anything runs.

A checksum confirms you got **exactly the file on the release page**. It can't tell whether the release itself is trustworthy, for example if a developer's GitHub account were hacked. Only track apps from developers you trust.

Thanks to the Reddit commenters whose feedback shaped the security rules and the move to a proper module.

## Upgrading from v1

v1 was a single script (`upgh`). Your app list carries over automatically. To clean up:

1. Remove the `upgh` line from your profile (`notepad $PROFILE`).
2. Delete the old script: `Remove-Item "$(Split-Path $PROFILE)\Scripts\Update-GitHubApps.ps1"`.
3. If you liked the short name, add your own alias: `Set-Alias upgh Update-GitHubApp` in your profile.

## Limitations

- Version checks use the numbers only, so `1.2.0-beta3` → `1.2.0-beta4` isn't detected as an update.
- Apps that report a wrong version in Installed apps will be offered the same update every time. Remove them with `Remove-GitHubApp`.
- `Find-GitHubApp` only finds apps that include a GitHub link in their uninstall information.

## Publishing a new version (maintainers)

1. Update `ModuleVersion` in `GitHubAppUpdater/GitHubAppUpdater.psd1`, then commit and push.
2. On GitHub, create a release with a matching tag (`v2.0.1` for version `2.0.1`).
3. A GitHub Action attaches `GitHubAppUpdater.zip` to the release, and publishes to the PowerShell Gallery if a repository secret named `PSGALLERY_API_KEY` is set.
4. Help lives in `docs/<language>/*.md`. After editing it, run `.\build\Build-Help.ps1` to rebuild the help, and commit both. To add a translation, copy `docs/en-US` to a folder like `docs/az-AZ`, translate the text (not the parameter names or code), and run the script.

## License

[MIT](LICENSE)
