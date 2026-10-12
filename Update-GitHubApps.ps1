<#
.SYNOPSIS
  Updates Windows apps that were installed from GitHub releases.

.DESCRIPTION
  Keeps a list of apps and their GitHub repos, compares each app's installed
  version with its latest GitHub release, and silently installs newer versions.

.EXAMPLE
  .\Update-GitHubApps.ps1                                        # check and update
  .\Update-GitHubApps.ps1 -CheckOnly                             # only report what's outdated
  .\Update-GitHubApps.ps1 -Discover                              # find GitHub apps automatically, then update
  .\Update-GitHubApps.ps1 -Add storytold/photocraft -Name PhotoCraft   # add an app to the list
  .\Update-GitHubApps.ps1 -List                                  # show the list
#>
[CmdletBinding()]
param(
    [switch]$CheckOnly,
    [switch]$Discover,
    [switch]$List,
    [string]$Add,
    [string]$Name,
    [string]$ConfigPath = (Join-Path $env:APPDATA 'GitHubAppUpdater\github-apps.json'),
    [switch]$Pause
)

$ErrorActionPreference = 'Stop'
$ProgressPreference    = 'SilentlyContinue'   # much faster downloads
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$UninstallPaths = @(
    'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*',
    'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*',
    'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
)
$Headers = @{ 'User-Agent' = 'github-app-updater'; 'Accept' = 'application/vnd.github+json' }
if ($env:GITHUB_TOKEN) { $Headers.Authorization = "Bearer $env:GITHUB_TOKEN" }

# ---------------- Helpers ----------------

function Read-Config {
    if (Test-Path $ConfigPath) {
        $raw = Get-Content $ConfigPath -Raw
        if ($raw -and $raw.Trim()) {
            # foreach unrolls the array the same way in PowerShell 7 and 5.1
            $parsed = ConvertFrom-Json -InputObject $raw
            foreach ($item in $parsed) { $item }
        }
    }
}

function Save-Config($Items) {
    $dir = Split-Path $ConfigPath
    if (-not (Test-Path $dir)) { New-Item $dir -ItemType Directory -Force | Out-Null }
    ConvertTo-Json -InputObject @($Items) -Depth 3 | Set-Content $ConfigPath -Encoding utf8
}

function Get-InstalledApps {
    Get-ItemProperty $UninstallPaths -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -and -not $_.SystemComponent }
}

function ConvertTo-Version([string]$Text) {
    if ($Text -notmatch '\d+(?:\.\d+){0,3}') { return $null }
    $v = $Matches[0]
    if ($v -notmatch '\.') { $v += '.0' }
    try { [version]$v } catch { $null }
}

function Get-LatestRelease([string]$Repo, [bool]$Prerelease) {
    if (-not $Prerelease) {
        try { return Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -Headers $Headers }
        catch { if ([int]$_.Exception.Response.StatusCode -ne 404) { throw } }
    }
    # No stable release (or prereleases wanted): take the newest non-draft release
    $all = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases?per_page=10" -Headers $Headers
    $all | Where-Object { -not $_.draft } | Select-Object -First 1
}

function Select-Asset($Release, [string]$Pattern, [bool]$WantMsi) {
    if ($Pattern) { return $Release.assets | Where-Object name -like $Pattern | Select-Object -First 1 }

    if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {
        $myArch    = '(?i)arm64|aarch64'
        $otherArch = '(?i)x64|amd64|x86_64|win64|ia32|i386|(?<!_)x86'
    } else {
        $myArch    = '(?i)x64|amd64|x86_64|win64'
        $otherArch = '(?i)arm64|aarch64|ia32|i386|(?<!_)x86(?!_64)|32-?bit'
    }

    $Release.assets |
        Where-Object { $_.name -match '(?i)\.(msi|exe)$' -and $_.name -notmatch '(?i)portable|mac|darwin|linux' } |
        ForEach-Object {
            $score = 0
            $isMsi = $_.name -match '(?i)\.msi$'
            if ($_.name -match $myArch) { $score += 10 }
            elseif ($_.name -match $otherArch) { $score -= 100 }
            if ($isMsi -eq $WantMsi) { $score += 20 }   # same installer type as what's installed
            if ($_.name -match '(?i)setup|install') { $score += 2 }
            [pscustomobject]@{ Asset = $_; Score = $score }
        } |
        Where-Object Score -ge 0 |
        Sort-Object Score -Descending |
        Select-Object -First 1 -ExpandProperty Asset
}

function Install-File([string]$File, [string]$SilentArgs) {
    if ($File -match '(?i)\.msi$') {
        $p = Start-Process msiexec.exe -ArgumentList "/i `"$File`" /qn /norestart" -Wait -PassThru
        return $p.ExitCode
    }
    if (-not $SilentArgs) {
        # Detect common installer types from the first few MB of the file
        $fs  = [IO.File]::OpenRead($File)
        $buf = New-Object byte[] ([Math]::Min(4MB, $fs.Length))
        [void]$fs.Read($buf, 0, $buf.Length)
        $fs.Close()
        $text = [Text.Encoding]::ASCII.GetString($buf)
        if     ($text -match 'Inno Setup') { $SilentArgs = '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART' }
        elseif ($text -match 'Nullsoft')   { $SilentArgs = '/S' }
    }
    if ($SilentArgs) {
        $p = Start-Process $File -ArgumentList $SilentArgs -Wait -PassThru
    } else {
        Write-Host '  Unknown installer type, opening it normally...' -ForegroundColor Yellow
        $p = Start-Process $File -Wait -PassThru
    }
    $p.ExitCode
}

function Find-GitHubApps {
    $wingetNames = @()
    if (Get-Command Get-WinGetPackage -ErrorAction SilentlyContinue) {
        # Skip apps winget already updates, so they aren't updated twice
        $wingetNames = @(Get-WinGetPackage | Where-Object Source -eq 'winget' | ForEach-Object Name)
    }
    foreach ($app in Get-InstalledApps) {
        $urls = @($app.URLInfoAbout, $app.URLUpdateInfo, $app.HelpLink) -join ' '
        if ($urls -match 'github\.com/([\w.-]+)/([\w.-]+)') {
            $owner = $Matches[1]
            $repo  = $Matches[2] -replace '\.git$', ''
            if ($owner -in 'sponsors', 'orgs', 'apps', 'features', 'settings') { continue }
            if ($app.DisplayName -in $wingetNames) { continue }
            [pscustomobject]@{ Name = $app.DisplayName; Repo = "$owner/$repo" }
        }
    }
}

function Exit-Script {
    if ($Pause) { Read-Host 'Press Enter to close' | Out-Null }
    exit
}

# ---------------- Commands that don't need admin ----------------

if ($Add) {
    $repo = $Add -replace '^https?://github\.com/', '' -replace '\.git$', '' -replace '/+$', ''
    if ($repo -notmatch '^[\w.-]+/[\w.-]+$') { Write-Host "Use the form owner/repo, e.g. storytold/photocraft" -ForegroundColor Red; exit 1 }
    if (-not $Name) { $Name = $repo.Split('/')[1] }
    $config = @(Read-Config)
    if ($config.Repo -contains $repo) { Write-Host "$repo is already in the list." -ForegroundColor Yellow; exit }
    $config += [pscustomobject]@{ Name = $Name; Repo = $repo }
    Save-Config $config
    Write-Host "Added $Name [$repo]" -ForegroundColor Green
    exit
}

if ($List) {
    $config = @(Read-Config)
    if (-not $config) { Write-Host 'The list is empty.' } else { $config | Format-Table Name, Repo -AutoSize }
    Write-Host "List file: $ConfigPath" -ForegroundColor DarkGray
    exit
}

# ---------------- Elevate once, so installers don't each ask for admin ----------------

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$isAdmin  = ([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin -and -not $CheckOnly) {
    $hostExe = (Get-Process -Id $PID).Path   # relaunch in the same PowerShell (7 or 5.1)
    $argList = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`"",
                 '-ConfigPath', "`"$ConfigPath`"", '-Pause')
    if ($Discover) { $argList += '-Discover' }
    Start-Process $hostExe -Verb RunAs -ArgumentList $argList
    exit
}

# ---------------- Main ----------------

$config = @(Read-Config)

if ($Discover) {
    Write-Host 'Scanning installed apps for GitHub links...' -ForegroundColor Cyan
    $added = 0
    foreach ($found in Find-GitHubApps) {
        if ($config.Repo -notcontains $found.Repo) {
            Write-Host "  + $($found.Name)  [$($found.Repo)]" -ForegroundColor Green
            $config += $found
            $added++
        }
    }
    if ($added) { Save-Config $config; Write-Host "  Added $added app(s)." }
    else        { Write-Host '  Nothing new found.' }
}

if (-not $config) {
    Write-Host 'No apps in the list yet. Add one with:' -ForegroundColor Yellow
    Write-Host '  -Add owner/repo -Name "App name"   or   -Discover'
    Exit-Script
}

$installed = @(Get-InstalledApps)
$updated = 0

foreach ($app in $config) {
    Write-Host "`n$($app.Name)  [$($app.Repo)]" -ForegroundColor Cyan
    try {
        $entry = $installed | Where-Object DisplayName -like "*$($app.Name)*" | Select-Object -First 1
        if (-not $entry) { Write-Host "  Not installed (no app named like '$($app.Name)'), skipping." -ForegroundColor DarkGray; continue }

        $release = Get-LatestRelease $app.Repo ([bool]$app.Prerelease)
        if (-not $release) { Write-Host '  No releases found.' -ForegroundColor DarkGray; continue }

        $current = ConvertTo-Version $entry.DisplayVersion
        $latest  = ConvertTo-Version $release.tag_name
        Write-Host "  Installed: $($entry.DisplayVersion)   Latest: $($release.tag_name)"
        if (-not $current -or -not $latest) { Write-Host "  Can't compare these versions, skipping." -ForegroundColor Yellow; continue }
        if ($current -ge $latest) { Write-Host '  Up to date.' -ForegroundColor Green; continue }

        $asset = Select-Asset $release $app.Pattern ($entry.WindowsInstaller -eq 1)
        if (-not $asset) { Write-Host '  No matching Windows installer in this release.' -ForegroundColor Yellow; continue }
        if ($CheckOnly)  { Write-Host "  Update available: $($asset.name)" -ForegroundColor Yellow; continue }

        $file = Join-Path $env:TEMP $asset.name
        Write-Host "  Downloading $($asset.name)..."
        Invoke-WebRequest $asset.browser_download_url -OutFile $file -Headers $Headers -UseBasicParsing

        if ($asset.digest) {
            $hash = (Get-FileHash $file -Algorithm SHA256).Hash
            if ("sha256:$hash" -ne $asset.digest) {
                Write-Host '  Hash mismatch! Not installing.' -ForegroundColor Red
                Remove-Item $file -Force
                continue
            }
            Write-Host '  Hash verified.'
        }

        Write-Host '  Installing...'
        $code = Install-File $file $app.SilentArgs
        Remove-Item $file -Force -ErrorAction SilentlyContinue
        if ($code -in 0, 3010, 1641) {
            Write-Host "  Updated to $($release.tag_name)." -ForegroundColor Green
            if ($code -ne 0) { Write-Host '  (A restart is needed to finish.)' -ForegroundColor Yellow }
            $updated++
        } else {
            Write-Host "  Installer exited with code $code." -ForegroundColor Red
        }
    } catch {
        Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
    }
}

Write-Host "`nDone. $updated app(s) updated." -ForegroundColor Cyan
Exit-Script
