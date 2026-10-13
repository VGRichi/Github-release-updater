<#
.SYNOPSIS
  Updates Windows apps and tools installed from GitHub releases.

.DESCRIPTION
  Keeps a list of apps and their GitHub repos, and installs newer releases.

  Installer apps (.msi / .exe) are compared with the version shown in
  Settings > Apps > Installed apps, and updated silently.

  Archive apps (.zip / .7z / .tar.gz), like DXVK or OptiScaler, are extracted
  into the folder(s) you choose. The script remembers which release it
  installed, backs up replaced files, and can keep files you've customized.

  Downloads with a GitHub checksum are verified and installed silently. Downloads
  without one are only installed after you confirm, or if you've marked the app as
  trusted. If an app's earlier releases had checksums and a new one doesn't, it
  always asks, because that's unusual.

.PARAMETER CheckOnly
  Only report what's outdated; install nothing. Doesn't need admin.
.PARAMETER Discover
  Scan installed apps for GitHub links and add new ones to the list.
.PARAMETER List
  Show the list.
.PARAMETER Add
  Add an app by owner/repo or full GitHub link.
.PARAMETER Name
  With -Add: the app's name. For installer apps, use the name shown in Installed apps (partial is fine).
.PARAMETER Destination
  With -Add: makes it an archive app and sets the folder its files are copied into.
.PARAMETER Subfolder
  With -Add: the folder inside the archive to copy from, e.g. x64 for DXVK.
.PARAMETER Pattern
  With -Add: which release file to download, e.g. '*.tar.gz'.
.PARAMETER AllowUnverified
  With -Add: trust this app, so releases without a checksum install without asking.
  This never skips the warning when an app that used to have checksums suddenly doesn't.
.PARAMETER Remove
  Remove apps from the list by name or repo.
.PARAMETER ConfigPath
  Use a different list file (handy for testing).

.EXAMPLE
  upgh
  Check every app in the list and install updates.
.EXAMPLE
  upgh -CheckOnly
  Show what's outdated without installing anything.
.EXAMPLE
  upgh -Add storytold/photocraft -Name PhotoCraft
  Track an app that was installed with an .msi or .exe.
.EXAMPLE
  upgh -Add doitsujin/dxvk -Name "DXVK (Witcher 3)" -Destination "D:\Games\Witcher 3\bin\x64" -Subfolder x64
  Track a zip/7z/tar.gz tool and extract its x64 folder into a game folder.
.EXAMPLE
  upgh -Discover
  Find installed apps that link to GitHub, add them, then update.
.EXAMPLE
  upgh -Remove PhotoCraft
  Stop tracking an app.
#>
[CmdletBinding()]
param(
    [switch]$CheckOnly,
    [switch]$Discover,
    [switch]$List,
    [string]$Add,
    [string]$Name,
    [string]$Destination,
    [string]$Subfolder,
    [string]$Pattern,
    [switch]$AllowUnverified,
    [string]$Remove,
    [ValidateNotNullOrEmpty()]
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

# ======================= Helpers: list file =======================

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
    ConvertTo-Json -InputObject @($Items) -Depth 4 | Set-Content $ConfigPath -Encoding utf8
}

# ======================= Helpers: GitHub =======================

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

function Get-VerifiedDownload($Asset, [string]$Folder) {
    $file = Join-Path $Folder $Asset.name
    Write-Host "  Downloading $($Asset.name)..."
    Invoke-WebRequest $Asset.browser_download_url -OutFile $file -Headers $Headers -UseBasicParsing
    if ($Asset.digest) {
        $hash = (Get-FileHash $file -Algorithm SHA256).Hash
        if ("sha256:$hash" -ne $Asset.digest) {
            Remove-Item $file -Force
            throw 'Hash mismatch! The download does not match the release. Nothing was installed.'
        }
        Write-Host '  Hash verified.'
    }
    $file
}

# ======================= Helpers: verification policy =======================
# Files with a checksum are verified and installed silently. Files without one are never
# installed silently unless you've marked the app as trusted, and never silently at all
# if earlier releases of the same app had checksums (that's unusual and worth a look).

function Set-AppField($App, [string]$Field, $Value) {
    $App | Add-Member -NotePropertyName $Field -NotePropertyValue $Value -Force
    $script:ConfigDirty = $true
}

function Get-VerificationNote($App, $Asset) {
    if ($Asset.digest)  { return '' }
    if ($App.HadDigest) { return '  (no checksum, but earlier releases had one: unusual)' }
    if ($App.AllowUnverified) { return '  (no checksum, app is trusted)' }
    '  (no checksum: will ask before installing)'
}

function Approve-Install($App, $Asset) {
    if ($Asset.digest) { return $true }   # verified during download

    if ($App.HadDigest) {
        Write-Host '  WARNING: earlier releases of this app had a checksum, but this file has none.' -ForegroundColor Red
        Write-Host '  That is unusual. Check the release page before installing.' -ForegroundColor Red
        $answer = "$(Read-Host '  Install anyway? [y] yes  [N] no')".Trim().ToLower()
        if ($answer -eq 'y') { return $true }
        Write-Host '  Skipped.' -ForegroundColor DarkGray
        return $false
    }

    if ($App.AllowUnverified) {
        Write-Host '  No checksum for this file. Installing because this app is marked as trusted.' -ForegroundColor Yellow
        return $true
    }

    Write-Host "  This file has no checksum, so the download can't be verified." -ForegroundColor Yellow
    $answer = "$(Read-Host '  Install anyway? [y] this time  [a] always for this app  [N] no')".Trim().ToLower()
    switch ($answer) {
        'y' { return $true }
        'a' { Set-AppField $App 'AllowUnverified' $true; return $true }
        default { Write-Host '  Skipped.' -ForegroundColor DarkGray; return $false }
    }
}

# ======================= Helpers: installer apps =======================

function Get-InstalledApps {
    Get-ItemProperty $UninstallPaths -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -and -not $_.SystemComponent }
}

function Select-InstallerAsset($Release, [string]$Pattern, [bool]$WantMsi) {
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

function Update-InstallerApp($App, $Release, $Entry) {
    $current = ConvertTo-Version $Entry.DisplayVersion
    $latest  = ConvertTo-Version $Release.tag_name
    Write-Host "  Installed: $($Entry.DisplayVersion)   Latest: $($Release.tag_name)"
    if (-not $current -or -not $latest) { Write-Host "  Can't compare these versions, skipping." -ForegroundColor Yellow; return $false }
    if ($current -ge $latest) { Write-Host '  Up to date.' -ForegroundColor Green; return $false }

    $asset = Select-InstallerAsset $Release $App.Pattern ($Entry.WindowsInstaller -eq 1)
    if (-not $asset) { Write-Host '  No matching Windows installer in this release.' -ForegroundColor Yellow; return $false }
    if ($asset.digest -and -not $App.HadDigest) { Set-AppField $App 'HadDigest' $true }
    if ($CheckOnly)  { Write-Host "  Update available: $($asset.name)$(Get-VerificationNote $App $asset)" -ForegroundColor Yellow; return $false }
    if (-not (Approve-Install $App $asset)) { return $false }

    $file = Get-VerifiedDownload $asset $env:TEMP
    Write-Host '  Installing...'
    $code = Install-File $file $App.SilentArgs
    Remove-Item $file -Force -ErrorAction SilentlyContinue
    if ($code -in 0, 3010, 1641) {
        Write-Host "  Updated to $($Release.tag_name)." -ForegroundColor Green
        if ($code -ne 0) { Write-Host '  (A restart is needed to finish.)' -ForegroundColor Yellow }
        return $true
    }
    Write-Host "  Installer exited with code $code." -ForegroundColor Red
    $false
}

# ======================= Helpers: archive apps =======================

function Select-ArchiveAsset($Release, [string]$Pattern) {
    if ($Pattern) { return $Release.assets | Where-Object name -like $Pattern | Select-Object -First 1 }
    $myArch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { '(?i)arm64|aarch64' } else { '(?i)x64|amd64|x86_64|win64' }
    $Release.assets |
        Where-Object {
            $_.name -match '(?i)\.(zip|7z|tar\.gz|tgz|tar\.xz)$' -and
            $_.name -notmatch '(?i)source|src|debug|pdb|symbols|linux|mac|darwin|native|steamrt'
        } |
        ForEach-Object {
            $score = 0
            if ($_.name -match $myArch) { $score += 10 }
            if ($_.name -match '(?i)win') { $score += 5 }
            [pscustomobject]@{ Asset = $_; Score = $score }
        } |
        Sort-Object Score -Descending |
        Select-Object -First 1 -ExpandProperty Asset
}

function Expand-AnyArchive([string]$File, [string]$Dest) {
    New-Item $Dest -ItemType Directory -Force | Out-Null
    if ($File -match '(?i)\.zip$') {
        Expand-Archive -LiteralPath $File -DestinationPath $Dest -Force
        return
    }
    $sevenZip = @("$env:ProgramFiles\7-Zip\7z.exe", "${env:ProgramFiles(x86)}\7-Zip\7z.exe") |
                Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1
    if ($File -match '(?i)\.7z$' -and $sevenZip) {
        & $sevenZip x $File "-o$Dest" -y | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "7-Zip could not extract $([IO.Path]::GetFileName($File))." }
        return
    }
    # Windows' built-in tar (not Git's) handles .tar.gz and, on recent Windows 11, often .7z
    $tar = Join-Path $env:SystemRoot 'System32\tar.exe'
    if (-not (Test-Path $tar)) { $tar = 'tar' }
    & $tar -xf $File -C $Dest | Out-Null
    if ($LASTEXITCODE -ne 0) {
        if ($File -match '(?i)\.7z$') { throw 'Could not extract this .7z file. Install 7-Zip (winget install --id 7zip.7zip) and try again.' }
        throw "Could not extract $([IO.Path]::GetFileName($File))."
    }
}

function Get-ContentRoot([string]$Extracted, [string]$Subfolder) {
    $root = $Extracted
    while ($true) {
        if ($Subfolder) {
            $direct = Join-Path $root $Subfolder
            if (Test-Path -LiteralPath $direct -PathType Container) { return $direct }
        }
        # Step into a single wrapper folder, e.g. dxvk-2.5/
        $items = @(Get-ChildItem -LiteralPath $root -Force)
        if ($items.Count -eq 1 -and $items[0].PSIsContainer) { $root = $items[0].FullName } else { break }
    }
    if (-not $Subfolder) { return $root }
    $leaf = Split-Path $Subfolder -Leaf
    if ((Split-Path $root -Leaf) -eq $leaf) { return $root }
    $found = Get-ChildItem -LiteralPath $root -Directory -Recurse | Where-Object Name -eq $leaf | Select-Object -First 1
    if ($found) { return $found.FullName }
    throw "Folder '$Subfolder' was not found inside the archive."
}

function Copy-ArchiveFiles($App, [string]$Source, [string]$Dest, [string]$BackupDir) {
    $include = @($App.Include) | Where-Object { $_ }
    $keep    = @($App.Keep)    | Where-Object { $_ }
    $renames = @{}
    if ($App.Rename) { foreach ($p in $App.Rename.PSObject.Properties) { $renames[$p.Name] = $p.Value } }

    $copied = 0
    foreach ($file in Get-ChildItem -LiteralPath $Source -File -Recurse) {
        $rel = $file.FullName.Substring($Source.Length).TrimStart('\', '/')
        if ($include -and -not ($include | Where-Object { $file.Name -like $_ -or $rel -like $_ })) { continue }

        $leaf      = if ($renames.ContainsKey($file.Name)) { $renames[$file.Name] } else { $file.Name }
        $relDir    = Split-Path $rel -Parent
        $relTarget = if ($relDir) { Join-Path $relDir $leaf } else { $leaf }
        $target    = Join-Path $Dest $relTarget

        if (Test-Path -LiteralPath $target) {
            # Files marked "Keep" (like your settings) are only added if missing, never overwritten
            if ($keep | Where-Object { $leaf -like $_ -or $file.Name -like $_ }) { continue }
            $backup = Join-Path $BackupDir $relTarget
            New-Item (Split-Path $backup) -ItemType Directory -Force | Out-Null
            Copy-Item -LiteralPath $target -Destination $backup -Force
        }
        New-Item (Split-Path $target) -ItemType Directory -Force | Out-Null
        Copy-Item -LiteralPath $file.FullName -Destination $target -Force
        $copied++
    }
    $copied
}

function Update-ArchiveApp($App, $Release) {
    $destinations = @(@($App.Destination) + @($App.Destinations) | Where-Object { $_ })
    if (-not $destinations) { Write-Host '  No Destination folder set for this app.' -ForegroundColor Yellow; return $false }
    $missing = @($destinations | Where-Object { -not (Test-Path -LiteralPath $_ -PathType Container) })
    if ($missing) { Write-Host "  Folder not found: $($missing -join ', ')" -ForegroundColor Yellow; return $false }

    $have = if ($App.InstalledVersion) { $App.InstalledVersion } else { 'unknown' }
    Write-Host "  Installed: $have   Latest: $($Release.tag_name)"
    if ($App.InstalledVersion -eq $Release.tag_name) { Write-Host '  Up to date.' -ForegroundColor Green; return $false }

    $asset = Select-ArchiveAsset $Release $App.Pattern
    if (-not $asset) { Write-Host '  No zip/7z/tar.gz file found in this release.' -ForegroundColor Yellow; return $false }
    if ($asset.digest -and -not $App.HadDigest) { Set-AppField $App 'HadDigest' $true }
    if ($CheckOnly)  { Write-Host "  Update available: $($asset.name)$(Get-VerificationNote $App $asset)" -ForegroundColor Yellow; return $false }
    if (-not (Approve-Install $App $asset)) { return $false }

    $work = Join-Path $env:TEMP ('ghupd-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
    New-Item $work -ItemType Directory | Out-Null
    try {
        $file = Get-VerifiedDownload $asset $work
        $extracted = Join-Path $work 'extracted'
        Expand-AnyArchive $file $extracted
        $source = Get-ContentRoot $extracted $App.Subfolder

        $safeName = $App.Name -replace '[\\/:*?"<>|]', '_'
        $stamp    = Get-Date -Format 'yyyy-MM-dd_HH-mm-ss'
        $i = 0
        foreach ($dest in $destinations) {
            $i++
            $backupDir = Join-Path (Split-Path $ConfigPath) "backups\$safeName\$stamp\$i"
            $n = Copy-ArchiveFiles $App $source $dest $backupDir
            Write-Host "  Copied $n file(s) to $dest"
            if (Test-Path $backupDir) { Write-Host "  Old files backed up to $backupDir" -ForegroundColor DarkGray }
        }
        Set-AppField $App 'InstalledVersion' $Release.tag_name
        Write-Host "  Updated to $($Release.tag_name)." -ForegroundColor Green
        return $true
    } finally {
        Remove-Item $work -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ======================= Discover =======================

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

# ======================= Commands that don't need admin =======================

if ($Add) {
    $repo = $Add -replace '^https?://github\.com/', '' -replace '\.git$', '' -replace '/+$', ''
    if ($repo -notmatch '^[\w.-]+/[\w.-]+$') { Write-Host 'Use the form owner/repo, e.g. storytold/photocraft' -ForegroundColor Red; exit 1 }
    if (-not $Name) { $Name = $repo.Split('/')[1] }

    $entry = [ordered]@{ Name = $Name; Repo = $repo }
    if ($Destination) {
        if (-not (Test-Path -LiteralPath $Destination -PathType Container)) { Write-Host "Folder not found: $Destination" -ForegroundColor Red; exit 1 }
        $Destination = (Resolve-Path -LiteralPath $Destination).Path
        $entry.Type = 'archive'
        $entry.Destination = $Destination
    }
    if ($Subfolder) { $entry.Subfolder = $Subfolder }
    if ($Pattern)   { $entry.Pattern   = $Pattern }
    if ($AllowUnverified) { $entry.AllowUnverified = $true }

    $config = @(Read-Config)
    if ($config | Where-Object { $_.Repo -eq $repo -and "$($_.Destination)" -eq "$Destination" }) {
        Write-Host "$repo is already in the list$(if ($Destination) { " for $Destination" })." -ForegroundColor Yellow
        exit
    }
    $config += [pscustomobject]$entry
    Save-Config $config
    Write-Host "Added $Name [$repo]$(if ($Destination) { " -> $Destination" })" -ForegroundColor Green
    exit
}

if ($Remove) {
    $config = @(Read-Config)
    $keepList = @($config | Where-Object { $_.Name -ne $Remove -and $_.Repo -ne $Remove })
    $removed = $config.Count - $keepList.Count
    if (-not $removed) { Write-Host "Nothing named '$Remove' in the list. Check the names with -List." -ForegroundColor Yellow; exit }
    Save-Config $keepList
    Write-Host "Removed $removed app(s)." -ForegroundColor Green
    exit
}

if ($List) {
    $config = @(Read-Config)
    if (-not $config) { Write-Host 'The list is empty.' }
    else {
        $config | Format-Table Name, Repo,
            @{ n = 'Type';      e = { if ($_.Type) { $_.Type } else { 'installer' } } },
            @{ n = 'Installed'; e = { $_.InstalledVersion } },
            Destination -AutoSize -Wrap
    }
    Write-Host "List file: $ConfigPath" -ForegroundColor DarkGray
    exit
}

# ======================= Elevate once, so installers don't each ask for admin =======================

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

# ======================= Main =======================

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

$installed = $null
$updated = 0
$script:ConfigDirty = $false

foreach ($app in $config) {
    Write-Host "`n$($app.Name)  [$($app.Repo)]" -ForegroundColor Cyan
    try {
        $isArchive = $app.Type -eq 'archive'
        $entry = $null
        if (-not $isArchive) {
            if ($null -eq $installed) { $installed = @(Get-InstalledApps) }
            $entry = $installed | Where-Object DisplayName -like "*$($app.Name)*" | Select-Object -First 1
            if (-not $entry) { Write-Host "  Not installed (no app named like '$($app.Name)'), skipping." -ForegroundColor DarkGray; continue }
        }

        $release = Get-LatestRelease $app.Repo ([bool]$app.Prerelease)
        if (-not $release) { Write-Host '  No releases found.' -ForegroundColor DarkGray; continue }

        if ($isArchive) {
            if (Update-ArchiveApp $app $release) { $updated++ }
        } else {
            if (Update-InstallerApp $app $release $entry) { $updated++ }
        }
    } catch {
        Write-Host "  Error: $($_.Exception.Message)" -ForegroundColor Red
    }
    if ($script:ConfigDirty) { Save-Config $config; $script:ConfigDirty = $false }
}

Write-Host "`nDone. $updated app(s) updated." -ForegroundColor Cyan
Exit-Script