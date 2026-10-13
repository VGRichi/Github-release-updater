#Requires -Version 5.1

# ============================================================================
#  Module state
# ============================================================================

$script:AppDataRoot = if ($env:APPDATA) { $env:APPDATA } else { [Environment]::GetFolderPath('ApplicationData') }
$script:DefaultConfigPath = Join-Path (Join-Path $script:AppDataRoot 'Reforge') 'apps.json'

# LEGACY, remove in 4.0: one-time move of the app list from the old name (GitHubAppUpdater v1/v2)
$legacyConfig = Join-Path (Join-Path $script:AppDataRoot 'GitHubAppUpdater') 'github-apps.json'
if (-not (Test-Path -LiteralPath $script:DefaultConfigPath) -and (Test-Path -LiteralPath $legacyConfig)) {
    try {
        New-Item -ItemType Directory -Path (Split-Path -Parent $script:DefaultConfigPath) -Force | Out-Null
        Copy-Item -LiteralPath $legacyConfig -Destination $script:DefaultConfigPath -ErrorAction Stop
    } catch {
        $script:DefaultConfigPath = $legacyConfig
    }
}
$script:ListChanged = $false
$script:UninstallPaths = @(
    'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*',
    'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*',
    'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
)

# ============================================================================
#  Private helpers: app list
# ============================================================================

function Read-AppList([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) { return }
    $raw = Get-Content -LiteralPath $Path -Raw
    if (-not ($raw -and $raw.Trim())) { return }
    foreach ($item in (ConvertFrom-Json -InputObject $raw)) {
        # LEGACY (GitHubAppUpdater v1/v2 lists), remove in 4.0: "Repo" is now "Repository",
        # and "Destinations" is merged into "Destination"
        if ($item.PSObject.Properties['Repo'] -and -not $item.PSObject.Properties['Repository']) {
            $item | Add-Member -NotePropertyName Repository -NotePropertyValue $item.Repo
            $item.PSObject.Properties.Remove('Repo')
        }
        if ($item.PSObject.Properties['Destinations']) {
            $item | Add-Member -NotePropertyName Destination -NotePropertyValue @(@($item.Destination) + @($item.Destinations) | Where-Object { $_ }) -Force
            $item.PSObject.Properties.Remove('Destinations')
        }
        $item
    }
}

function Save-AppList([string]$Path, $Items) {
    # Write a temp file and swap it in, so a crash or full disk never leaves a half-written list
    $Path = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Path)
    $dir = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir -Force -WhatIf:$false | Out-Null }
    $temp = "$Path.tmp"
    ConvertTo-Json -InputObject @($Items) -Depth 5 | Set-Content -LiteralPath $temp -Encoding utf8 -WhatIf:$false
    if (Test-Path -LiteralPath $Path) { [IO.File]::Replace($temp, $Path, [NullString]::Value) }   # no backup file
    else { [IO.File]::Move($temp, $Path) }
}

function Set-AppField($App, [string]$Field, $Value) {
    $App | Add-Member -NotePropertyName $Field -NotePropertyValue $Value -Force
    $script:ListChanged = $true
}

function Get-AppDestination($App) {
    @($App.Destination | Where-Object { $_ })
}

function ConvertTo-AppView($App) {
    $isArchive = $App.Type -eq 'archive'
    [pscustomobject]@{
        PSTypeName       = 'Reforge.App'
        Name             = $App.Name
        Repository       = $App.Repository
        Type             = if ($isArchive) { 'Archive' } else { 'Installer' }
        Destination      = (Get-AppDestination $App) -join '; '
        InstalledVersion = $App.InstalledVersion
        Trusted          = [bool]$App.AllowUnverified
    }
}

# ============================================================================
#  Private helpers: GitHub
# ============================================================================

function Get-RequestHeader([securestring]$Token) {
    $headers = @{ 'User-Agent' = 'Reforge'; 'Accept' = 'application/vnd.github+json' }
    $plain = if ($Token) { [Net.NetworkCredential]::new('', $Token).Password } elseif ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN }
    if ($plain) { $headers.Authorization = "Bearer $plain" }
    $headers
}

function Get-LatestRelease([string]$Repository, [bool]$Prerelease, [hashtable]$Headers) {
    if (-not $Prerelease) {
        try { return Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest" -Headers $Headers }
        catch { if ([int]$_.Exception.Response.StatusCode -ne 404) { throw } }
    }
    # No stable release (or prereleases wanted): take the newest non-draft release
    $all = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases?per_page=10" -Headers $Headers
    $all | Where-Object { -not $_.draft } | Select-Object -First 1
}

function New-WorkFolder {
    $path = Join-Path ([IO.Path]::GetTempPath()) ('reforge-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
    New-Item -ItemType Directory -Path $path -WhatIf:$false | Out-Null
    $path
}

function Save-ReleaseAsset($Asset, [string]$Folder) {
    $ProgressPreference = 'SilentlyContinue'   # much faster downloads in Windows PowerShell
    $file = Join-Path $Folder $Asset.name
    Write-Verbose "Downloading $($Asset.browser_download_url)"
    Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $file -UseBasicParsing -Headers @{ 'User-Agent' = 'Reforge' }
    if ($Asset.digest) {
        $hash = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash
        if ("sha256:$hash" -ne $Asset.digest) {
            Remove-Item -LiteralPath $file -Force -WhatIf:$false
            throw "Checksum mismatch for $($Asset.name). Nothing was installed."
        }
        Write-Verbose 'Checksum verified.'
    }
    $file
}

function ConvertTo-AppVersion([string]$Text) {
    if ($Text -notmatch '\d+(?:\.\d+){0,3}') { return $null }
    $v = $Matches[0]
    if ($v -notmatch '\.') { $v += '.0' }
    try { [version]$v } catch { $null }
}

function Test-PerUserInstall($Program) {
    # Apps installed only for the current user are registered under HKEY_CURRENT_USER
    # and can be updated without admin rights
    [string]$Program.PSPath -match 'HKEY_CURRENT_USER'
}
function Test-IsAdmin {
    try {
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        ([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    } catch { $false }
}

# ============================================================================
#  Private helpers: verification policy
#  With a checksum: verified, installed without asking.
#  Without one: asks, unless the app is trusted (AllowUnverified).
#  Without one but earlier releases had one: always asks (that's unusual).
# ============================================================================

function Read-Choice([string]$Caption, [string]$Message, [string[]]$Labels, [int]$Default) {
    $choices = [System.Management.Automation.Host.ChoiceDescription[]]@($Labels | ForEach-Object { [System.Management.Automation.Host.ChoiceDescription]::new($_) })
    $Host.UI.PromptForChoice($Caption, $Message, $choices, $Default)
}

function Get-VerificationNote($App, $Asset) {
    if ($Asset.digest)        { return 'Checksum available' }
    if ($App.HadDigest)       { return 'No checksum (unusual: earlier releases had one)' }
    if ($App.AllowUnverified) { return 'No checksum (trusted app)' }
    'No checksum (will ask before installing)'
}

function Approve-Install($App, $Asset) {
    if ($Asset.digest) { return $true }

    if ($App.HadDigest) {
        Write-Warning "$($App.Name): earlier releases had a checksum, but $($Asset.name) has none. That is unusual; check the release page before installing."
        return (Read-Choice 'Unverified download' "Install $($Asset.name) anyway?" @('&Yes', '&No') 1) -eq 0
    }

    if ($App.AllowUnverified) {
        Write-Verbose "No checksum for $($Asset.name); installing because $($App.Name) is trusted."
        return $true
    }

    $answer = Read-Choice 'Unverified download' "$($Asset.name) has no checksum, so it can't be verified. Install anyway? (Always = trust this app from now on)" @('&Yes', '&Always', '&No') 2
    switch ($answer) {
        0 { $true }
        1 { Set-AppField $App 'AllowUnverified' $true; $true }
        default { $false }
    }
}

# ============================================================================
#  Private helpers: installer apps (.msi / .exe)
# ============================================================================

function Get-InstalledProgram {
    Get-ItemProperty -Path $script:UninstallPaths -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -and -not $_.SystemComponent }
}

function Get-ProgramBaseName([string]$Name) {
    # "PhotoCraft 1.2.3 (x64)" -> "PhotoCraft (x64)": installed names often include the version,
    # which changes with every update
    (($Name -replace '(?i)(?<![\w.])v?\d+(?:\.\d+)+[\w.+-]*', '') -replace '\s{2,}', ' ').Trim()
}

function Find-InstalledProgram($Programs, [string]$Name) {
    # The name as part of the installed name, or else the same name once version numbers are left out
    $found = $Programs | Where-Object DisplayName -like "*$Name*" | Select-Object -First 1
    if ($found) { return $found }
    $base = Get-ProgramBaseName $Name
    if ($base) { $Programs | Where-Object { (Get-ProgramBaseName $_.DisplayName) -eq $base } | Select-Object -First 1 }
}

function Get-ArchPattern {
    # How release file names spell this PC's architecture, and the ones it can't use
    if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {
        @{ Mine = '(?i)arm64|aarch64'; Other = '(?i)x64|amd64|x86_64|win64|ia32|i386|(?<!_)x86' }
    } else {
        @{ Mine = '(?i)x64|amd64|x86_64|win64'; Other = '(?i)arm64|aarch64|ia32|i386|(?<!_)x86(?!_64)|32-?bit' }
    }
}

function Test-NameHasWord([string]$Name, [string[]]$Word) {
    # Not inside a longer word, so "emacs" doesn't count as "mac" (but "linux64" counts as "linux")
    $Name -match ('(?i)(?<![a-z])(' + ($Word -join '|') + ')(?![a-z])')
}

function Select-InstallerAsset($Release, [string]$Pattern, [bool]$WantMsi) {
    if ($Pattern) { return $Release.assets | Where-Object name -like $Pattern | Select-Object -First 1 }
    $arch = Get-ArchPattern
    $Release.assets |
        Where-Object { $_.name -match '(?i)\.(msi|exe)$' -and -not (Test-NameHasWord $_.name 'portable', 'mac', 'macos', 'osx', 'darwin', 'linux') } |
        ForEach-Object {
            $score = 0
            if ($_.name -match $arch.Mine) { $score += 10 }
            elseif ($_.name -match $arch.Other) { $score -= 100 }
            if (($_.name -match '(?i)\.msi$') -eq $WantMsi) { $score += 20 }   # same installer type as what's installed
            if ($_.name -match '(?i)setup|install') { $score += 2 }
            [pscustomobject]@{ Asset = $_; Score = $score }
        } |
        Where-Object Score -ge 0 |
        Sort-Object Score -Descending |
        Select-Object -First 1 -ExpandProperty Asset
}

function Invoke-Installer([string]$File, [string]$SilentArgs) {
    if ($File -match '(?i)\.msi$') {
        $p = Start-Process -FilePath msiexec.exe -ArgumentList "/i `"$File`" /qn /norestart" -Wait -PassThru
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
        $p = Start-Process -FilePath $File -ArgumentList $SilentArgs -Wait -PassThru
    } else {
        Write-Warning "Unknown installer type for $([IO.Path]::GetFileName($File)); opening it normally."
        $p = Start-Process -FilePath $File -Wait -PassThru
    }
    $p.ExitCode
}

function Invoke-InstallerAppUpdate($App, $Release, $Program, $Result, $Cmdlet) {
    $Result.Installed = $Program.DisplayVersion
    $current = ConvertTo-AppVersion $Program.DisplayVersion
    $latest  = ConvertTo-AppVersion $Release.tag_name
    if (-not $current -or -not $latest) { $Result.Status = 'Skipped'; $Result.Note = "Can't compare versions"; return }
    if ($current -ge $latest) { $Result.Status = 'UpToDate'; return }

    $asset = Select-InstallerAsset $Release $App.Pattern ($Program.WindowsInstaller -eq 1)
    if (-not $asset) { $Result.Status = 'Skipped'; $Result.Note = 'No matching Windows installer in this release'; return }
    if ($asset.digest -and -not $App.HadDigest) { Set-AppField $App 'HadDigest' $true }

    $Result.Status = 'UpdateAvailable'
    $Result.Note = Get-VerificationNote $App $asset
    if (-not $Cmdlet.ShouldProcess("$($App.Name) $($Release.tag_name)", "Install $($asset.name)")) { return }
    if (-not (Test-PerUserInstall $Program) -and -not (Test-IsAdmin)) { $Result.Status = 'NeedsAdmin'; $Result.Note = 'Installed for all users; run PowerShell as administrator'; return }    if (-not (Approve-Install $App $asset)) { $Result.Status = 'Skipped'; $Result.Note = 'Unverified download declined'; return }

    $work = New-WorkFolder
    try {
        $file = Save-ReleaseAsset $asset $work
        Write-Verbose "Installing $($asset.name)"
        $code = Invoke-Installer $file $App.SilentArgs
        if ($code -in 0, 3010, 1641) {
            $Result.Status = 'Updated'
            $Result.Note = if ($code -ne 0) { 'Restart needed to finish' } elseif ($asset.digest) { 'Checksum verified' } else { 'Unverified' }
        } else {
            $Result.Status = 'Failed'
            $Result.Note = "Installer exit code $code"
        }
    } finally {
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue -WhatIf:$false
    }
}

# ============================================================================
#  Private helpers: archive apps (.zip / .7z / .tar.gz)
# ============================================================================

function Select-ArchiveAsset($Release, [string]$Pattern) {
    if ($Pattern) { return $Release.assets | Where-Object name -like $Pattern | Select-Object -First 1 }
    $arch = Get-ArchPattern
    $Release.assets |
        Where-Object {
            $_.name -match '(?i)\.(zip|7z|tar\.gz|tgz|tar\.xz)$' -and
            -not (Test-NameHasWord $_.name 'source', 'src', 'debug', 'pdb', 'symbols', 'linux', 'mac', 'macos', 'osx', 'darwin', 'native', 'steamrt')
        } |
        ForEach-Object {
            $score = 0
            if ($_.name -match $arch.Mine) { $score += 10 }
            elseif ($_.name -match $arch.Other) { $score -= 100 }
            if ($_.name -match '(?i)win') { $score += 5 }
            [pscustomobject]@{ Asset = $_; Score = $score }
        } |
        Where-Object Score -ge 0 |
        Sort-Object Score -Descending |
        Select-Object -First 1 -ExpandProperty Asset
}

function Expand-ReleaseArchive([string]$File, [string]$Destination) {
    $ProgressPreference = 'SilentlyContinue'   # hide per-file progress; the main progress bar stays
    New-Item -ItemType Directory -Path $Destination -Force -WhatIf:$false | Out-Null
    if ($File -match '(?i)\.zip$') {
        Expand-Archive -LiteralPath $File -DestinationPath $Destination -Force
        return
    }
    $sevenZip = @("$env:ProgramFiles\7-Zip\7z.exe", "${env:ProgramFiles(x86)}\7-Zip\7z.exe") |
                Where-Object { $_ -and (Test-Path -LiteralPath $_) } | Select-Object -First 1
    if ($File -match '(?i)\.7z$' -and $sevenZip) {
        & $sevenZip x $File "-o$Destination" -y | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "7-Zip could not extract $([IO.Path]::GetFileName($File))." }
        return
    }
    # Windows' built-in tar (not Git's) handles .tar.gz and, on recent Windows 11, often .7z
    $tar = 'tar'
    if ($env:SystemRoot) {
        $systemTar = Join-Path $env:SystemRoot 'System32\tar.exe'
        if (Test-Path -LiteralPath $systemTar) { $tar = $systemTar }
    }
    & $tar -xf $File -C $Destination | Out-Null
    if ($LASTEXITCODE -ne 0) {
        if ($File -match '(?i)\.7z$') { throw 'Could not extract this .7z file. Install 7-Zip (winget install --id 7zip.7zip) and try again.' }
        throw "Could not extract $([IO.Path]::GetFileName($File))."
    }
}

function Get-ArchiveContentRoot([string]$Extracted, [string]$Subfolder) {
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
    $leaf = Split-Path -Leaf $Subfolder
    if ((Split-Path -Leaf $root) -eq $leaf) { return $root }
    $found = Get-ChildItem -LiteralPath $root -Directory -Recurse | Where-Object Name -eq $leaf | Select-Object -First 1
    if ($found) { return $found.FullName }
    throw "Folder '$Subfolder' was not found inside the archive."
}

function Copy-ArchiveContent($App, [string]$Source, [string]$Destination, [string]$BackupDir, [System.Collections.Generic.List[object]]$Journal) {
    # Every replaced or added file goes into $Journal, so Undo-ArchiveContent can put things back
    $ProgressPreference = 'SilentlyContinue'   # hide per-file progress; the main progress bar stays
    $include = @($App.Include) | Where-Object { $_ }
    $keep    = @($App.Keep)    | Where-Object { $_ }
    $renames = @{}
    if ($App.Rename -is [System.Collections.IDictionary]) {
        foreach ($k in $App.Rename.Keys) { $renames[[string]$k] = [string]$App.Rename[$k] }
    } elseif ($App.Rename) {
        foreach ($p in $App.Rename.PSObject.Properties) { $renames[$p.Name] = [string]$p.Value }
    }

    $copied = 0
    foreach ($file in Get-ChildItem -LiteralPath $Source -File -Recurse) {
        $rel = $file.FullName.Substring($Source.Length).TrimStart('\', '/')
        if ($include -and -not ($include | Where-Object { $file.Name -like $_ -or $rel -like $_ })) { continue }

        $leaf      = if ($renames.ContainsKey($file.Name)) { $renames[$file.Name] } else { $file.Name }
        $relDir    = Split-Path -Parent $rel
        $relTarget = if ($relDir) { Join-Path $relDir $leaf } else { $leaf }
        $target    = Join-Path $Destination $relTarget

        $backup = $null
        if (Test-Path -LiteralPath $target) {
            # Files marked "Keep" (like your settings) are only added if missing, never overwritten
            if ($keep | Where-Object { $leaf -like $_ -or $file.Name -like $_ -or $relTarget -like $_ -or $rel -like $_ }) { continue }
            $backup = Join-Path $BackupDir $relTarget
            New-Item -ItemType Directory -Path (Split-Path -Parent $backup) -Force -WhatIf:$false | Out-Null
            Copy-Item -LiteralPath $target -Destination $backup -Force -WhatIf:$false -ErrorAction Stop
        }
        New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force -WhatIf:$false | Out-Null
        Copy-Item -LiteralPath $file.FullName -Destination $target -Force -WhatIf:$false -ErrorAction Stop
        if ($null -ne $Journal) { $Journal.Add([pscustomobject]@{ Target = $target; Backup = $backup }) }
        $copied++
    }
    $copied
}

function Undo-ArchiveContent([System.Collections.Generic.List[object]]$Journal) {
    # Newest first: restore replaced files from their backup, delete added ones
    for ($i = $Journal.Count - 1; $i -ge 0; $i--) {
        $entry = $Journal[$i]
        try {
            if ($entry.Backup) { Copy-Item -LiteralPath $entry.Backup -Destination $entry.Target -Force -WhatIf:$false -ErrorAction Stop }
            else { Remove-Item -LiteralPath $entry.Target -Force -WhatIf:$false -ErrorAction Stop }
        } catch {
            Write-Warning "Couldn't put back $($entry.Target): $($_.Exception.Message)"
        }
    }
}

function Invoke-ArchiveAppUpdate($App, $Release, $Result, $Cmdlet, [string]$ConfigPath) {
    $Result.Installed = $App.InstalledVersion
    $destinations = Get-AppDestination $App
    if (-not $destinations) { $Result.Status = 'Skipped'; $Result.Note = 'No destination folder set'; return }
    $missing = @($destinations | Where-Object { -not (Test-Path -LiteralPath $_ -PathType Container) })
    if ($missing) { $Result.Status = 'Skipped'; $Result.Note = "Folder not found: $($missing -join ', ')"; return }
    if ($App.InstalledVersion -eq $Release.tag_name) { $Result.Status = 'UpToDate'; return }

    $asset = Select-ArchiveAsset $Release $App.Pattern
    if (-not $asset) { $Result.Status = 'Skipped'; $Result.Note = 'No zip/7z/tar.gz file in this release'; return }
    if ($asset.digest -and -not $App.HadDigest) { Set-AppField $App 'HadDigest' $true }

    $Result.Status = 'UpdateAvailable'
    $Result.Note = Get-VerificationNote $App $asset
    if (-not $Cmdlet.ShouldProcess("$($App.Name) $($Release.tag_name)", "Extract $($asset.name) into $($destinations -join ', ')")) { return }
    if (-not (Approve-Install $App $asset)) { $Result.Status = 'Skipped'; $Result.Note = 'Unverified download declined'; return }

    $work = New-WorkFolder
    try {
        $file = Save-ReleaseAsset $asset $work
        $extracted = Join-Path $work 'extracted'
        Expand-ReleaseArchive $file $extracted
        $source = Get-ArchiveContentRoot $extracted $App.Subfolder

        $safeName = $App.Name -replace '[\\/:*?"<>|]', '_'
        $stamp    = Get-Date -Format 'yyyy-MM-dd_HH-mm-ss'
        $backupRoot = Join-Path (Join-Path (Join-Path (Split-Path -Parent $ConfigPath) 'backups') $safeName) $stamp
        $journal = [System.Collections.Generic.List[object]]::new()
        $i = 0
        try {
            foreach ($dest in $destinations) {
                $i++
                $n = Copy-ArchiveContent $App $source $dest (Join-Path $backupRoot $i) $journal
                Write-Verbose "Copied $n file(s) to $dest"
            }
        } catch {
            # All or nothing: a half-updated folder (often a file in use by a running game) is worse than the old version
            Undo-ArchiveContent $journal
            throw "Couldn't update $dest (is the app running?): $($_.Exception.Message) Files already replaced were put back."
        }
        if (Test-Path -LiteralPath $backupRoot) { Write-Verbose "Replaced files were backed up to $backupRoot" }

        Set-AppField $App 'InstalledVersion' $Release.tag_name
        $Result.Installed = $Release.tag_name
        $Result.Status = 'Updated'
        $Result.Note = if ($asset.digest) { 'Checksum verified' } else { 'Unverified' }
    } finally {
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue -WhatIf:$false
    }
}

# ============================================================================
#  Public commands
# ============================================================================

function Get-ReforgeApp {
    [CmdletBinding()]
    [OutputType('Reforge.App')]
    param(
        [Parameter(Position = 0)]
        [SupportsWildcards()]
        [string[]]$Name = '*',

        [ValidateNotNullOrEmpty()][string]$ConfigPath = $script:DefaultConfigPath
    )
    foreach ($app in Read-AppList $ConfigPath) {
        if ($Name | Where-Object { $app.Name -like $_ }) { ConvertTo-AppView $app }
    }
}

function Add-ReforgeApp {
    [CmdletBinding()]
    [OutputType('Reforge.App')]
    param(
        [Parameter(Mandatory, Position = 0, ValueFromPipelineByPropertyName)]
        [Alias('Repo')]   # LEGACY: GitHubAppUpdater v1 lists and CSVs used Repo
        [string]$Repository,

        [Parameter(Position = 1, ValueFromPipelineByPropertyName)]
        [string]$Name,

        [Parameter(ValueFromPipelineByPropertyName)]
        [string[]]$Destination,

        [Parameter(ValueFromPipelineByPropertyName)]
        [string]$Subfolder,

        [Parameter(ValueFromPipelineByPropertyName)]
        [string]$Pattern,

        [Parameter(ValueFromPipelineByPropertyName)]
        [string[]]$Include,

        [Parameter(ValueFromPipelineByPropertyName)]
        [string[]]$Keep,

        [Parameter(ValueFromPipelineByPropertyName)]
        [object]$Rename,

        [Parameter(ValueFromPipelineByPropertyName)]
        [string]$SilentArgs,

        [Parameter(ValueFromPipelineByPropertyName)]
        [switch]$Prerelease,

        [Parameter(ValueFromPipelineByPropertyName)]
        [switch]$AllowUnverified,

        [switch]$Force,

        [switch]$PassThru,

        [ValidateNotNullOrEmpty()][string]$ConfigPath = $script:DefaultConfigPath
    )
    begin {
        $apps = [System.Collections.Generic.List[object]]::new()
        foreach ($a in Read-AppList $ConfigPath) { $apps.Add($a) }
        $changed = $false
    }
    process {
        $repo = $Repository.Trim() -replace '^https?://(www\.)?github\.com/', '' -replace '\.git$', '' -replace '/+$', ''
        if ($repo -notmatch '^[\w.-]+/[\w.-]+$') {
            Write-Error "'$Repository' isn't a GitHub repository. Use owner/repo, for example storytold/photocraft."
            return
        }
        $appName = if ($Name) { $Name } else { $repo.Split('/')[1] }

        $resolved = @()
        foreach ($d in @($Destination | Where-Object { $_ })) {
            if (-not (Test-Path -LiteralPath $d -PathType Container)) { Write-Error "Folder not found: $d"; return }
            $resolved += (Resolve-Path -LiteralPath $d).ProviderPath
        }

        $index = -1
        for ($i = 0; $i -lt $apps.Count; $i++) { if ($apps[$i].Name -eq $appName) { $index = $i; break } }
        if ($index -ge 0 -and -not $Force) {
            Write-Error "'$appName' is already in the list. Use a different -Name, or -Force to replace it."
            return
        }

        $entry = [ordered]@{ Name = $appName; Repository = $repo }
        if ($resolved)        { $entry.Type = 'archive'; $entry.Destination = $resolved }
        if ($Subfolder)       { $entry.Subfolder = $Subfolder }
        if ($Pattern)         { $entry.Pattern = $Pattern }
        if ($Include)         { $entry.Include = $Include }
        if ($Keep)            { $entry.Keep = $Keep }
        if ($Rename)          { $entry.Rename = $Rename }
        if ($SilentArgs)      { $entry.SilentArgs = $SilentArgs }
        if ($Prerelease)      { $entry.Prerelease = $true }
        if ($AllowUnverified) { $entry.AllowUnverified = $true }

        if ($index -ge 0) {
            # Replacing the same repo: keep what we've learned about it
            $old = $apps[$index]
            if ($old.Repository -eq $repo) {
                if ($old.HadDigest) { $entry.HadDigest = $true }
                if ($old.InstalledVersion -and $resolved) { $entry.InstalledVersion = $old.InstalledVersion }
            }
            $apps[$index] = [pscustomobject]$entry
        } else {
            $apps.Add([pscustomobject]$entry)
        }
        $changed = $true
        Write-Verbose "Added $appName [$repo]"
        if ($PassThru) { ConvertTo-AppView ([pscustomobject]$entry) }
    }
    end {
        if ($changed) { Save-AppList $ConfigPath $apps }
    }
}

function Remove-ReforgeApp {
    [CmdletBinding(SupportsShouldProcess, ConfirmImpact = 'Medium')]
    param(
        [Parameter(Mandatory, Position = 0, ValueFromPipelineByPropertyName)]
        [SupportsWildcards()]
        [string[]]$Name,

        [ValidateNotNullOrEmpty()][string]$ConfigPath = $script:DefaultConfigPath
    )
    begin {
        $apps = [System.Collections.Generic.List[object]]::new()
        foreach ($a in Read-AppList $ConfigPath) { $apps.Add($a) }
        $changed = $false
    }
    process {
        foreach ($n in $Name) {
            $found = @($apps | Where-Object { $_.Name -like $n })
            if (-not $found) { Write-Error "No app named '$n' in the list. Check the names with Get-ReforgeApp."; continue }
            foreach ($app in $found) {
                if ($PSCmdlet.ShouldProcess($app.Name, 'Stop tracking')) {
                    [void]$apps.Remove($app)
                    $changed = $true
                }
            }
        }
    }
    end {
        if ($changed) { Save-AppList $ConfigPath $apps }
    }
}

function Find-ReforgeApp {
    [CmdletBinding()]
    [OutputType('Reforge.Candidate')]
    param(
        [ValidateNotNullOrEmpty()][string]$ConfigPath = $script:DefaultConfigPath
    )
    $tracked = @(Read-AppList $ConfigPath | ForEach-Object { $_.Repository })
    $wingetNames = @()
    if (Get-Command -Name Get-WinGetPackage -ErrorAction SilentlyContinue) {
        # Skip apps winget already updates, so they aren't updated twice
        $wingetNames = @(Get-WinGetPackage | Where-Object Source -eq 'winget' | ForEach-Object Name)
    }
    $seen = @{}
    foreach ($program in Get-InstalledProgram) {
        $urls = @($program.URLInfoAbout, $program.URLUpdateInfo, $program.HelpLink) -join ' '
        if ($urls -notmatch 'github\.com/([\w.-]+)/([\w.-]+)') { continue }
        $owner = $Matches[1]
        $repo  = "$owner/$($Matches[2] -replace '\.git$', '')"
        if ($owner -in 'sponsors', 'orgs', 'apps', 'features', 'settings') { continue }
        if ($tracked -contains $repo -or $wingetNames -contains $program.DisplayName -or $seen[$repo]) { continue }
        $seen[$repo] = $true
        [pscustomobject]@{
            PSTypeName = 'Reforge.Candidate'
            Name       = Get-ProgramBaseName $program.DisplayName   # stays valid after the version changes
            Repository = $repo
            Version    = $program.DisplayVersion
        }
    }
}

function Update-ReforgeApp {
    [CmdletBinding(SupportsShouldProcess)]
    [OutputType('Reforge.Result')]
    param(
        [Parameter(Position = 0, ValueFromPipelineByPropertyName)]
        [SupportsWildcards()]
        [string[]]$Name,

        [securestring]$Token,

        [ValidateNotNullOrEmpty()][string]$ConfigPath = $script:DefaultConfigPath
    )
    begin {
        $requested = [System.Collections.Generic.List[string]]::new()
    }
    process {
        foreach ($n in $Name) { if ($n) { $requested.Add($n) } }
    }
    end {
        $apps = @(Read-AppList $ConfigPath)
        if (-not $apps) {
            Write-Warning 'No apps in the list yet. Add one with Add-ReforgeApp, or find installed ones with Find-ReforgeApp.'
            return
        }
        # @() keeps a single app an array; Windows PowerShell has no .Count on a lone object
        $targets = @(if ($requested.Count) { $apps | Where-Object { $app = $_; $requested | Where-Object { $app.Name -like $_ } } } else { $apps })
        foreach ($n in $requested) {
            if (-not ($apps | Where-Object { $_.Name -like $n })) { Write-Error "No app named '$n' in the list." }
        }

        $headers = Get-RequestHeader $Token
        $programs = $null
        $script:ListChanged = $false
        $i = 0

        foreach ($app in $targets) {
            $i++
            Write-Progress -Activity 'Updating Reforge apps' -Status $app.Name -PercentComplete (($i - 1) / $targets.Count * 100)
            $result = [ordered]@{ Name = $app.Name; Repository = $app.Repository; Installed = $null; Latest = $null; Status = $null; Note = $null }
            try {
                $isArchive = $app.Type -eq 'archive'
                $program = $null
                if (-not $isArchive) {
                    if ($null -eq $programs) { $programs = @(Get-InstalledProgram) }
                    $program = Find-InstalledProgram $programs $app.Name
                }

                if (-not $isArchive -and -not $program) {
                    $result.Status = 'NotInstalled'
                    $result.Note = "No installed app named like '$($app.Name)'"
                } else {
                    $release = Get-LatestRelease $app.Repository ([bool]$app.Prerelease) $headers
                    if (-not $release) {
                        $result.Status = 'Skipped'
                        $result.Note = 'No releases found'
                    } else {
                        $result.Latest = $release.tag_name
                        if ($isArchive) { Invoke-ArchiveAppUpdate $app $release $result $PSCmdlet $ConfigPath }
                        else            { Invoke-InstallerAppUpdate $app $release $program $result $PSCmdlet }
                    }
                }
            } catch {
                $result.Status = 'Failed'
                $result.Note = $_.Exception.Message
                Write-Warning "$($app.Name): $($_.Exception.Message)"
            }
            # -WhatIf changes nothing on disk, not even what it learned (like HadDigest)
            if ($script:ListChanged -and -not $WhatIfPreference) { Save-AppList $ConfigPath $apps }
            $script:ListChanged = $false
            $out = [pscustomobject]$result
            $out.PSObject.TypeNames.Insert(0, 'Reforge.Result')
            $out
        }
        Write-Progress -Activity 'Updating Reforge apps' -Completed
    }
}

Export-ModuleMember -Function Get-ReforgeApp, Add-ReforgeApp, Remove-ReforgeApp, Find-ReforgeApp, Update-ReforgeApp
