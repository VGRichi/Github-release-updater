#Requires -Version 5.1

# ============================================================================
#  Module state
# ============================================================================

$script:AppDataRoot = if ($env:APPDATA) { $env:APPDATA } else { [Environment]::GetFolderPath('ApplicationData') }
$script:DefaultConfigPath = Join-Path (Join-Path $script:AppDataRoot 'Reforge') 'apps.json'
$script:ListChanged = $false
$script:BackupsToKeep = 3                      # per app; older backup folders are deleted
$script:WorkRoot = [IO.Path]::GetTempPath()    # downloads are extracted in reforge-* folders here
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
    # foreach unrolls the array; Windows PowerShell's ConvertFrom-Json returns it as one object
    foreach ($item in (ConvertFrom-Json -InputObject $raw)) { $item }
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

function New-Result($App) {
    [ordered]@{ Name = $App.Name; Repository = $App.Repository; Installed = $null; Latest = $null; Status = $null; Note = $null }
}

function ConvertTo-ResultView($Result) {
    $out = [pscustomobject]$Result
    $out.PSObject.TypeNames.Insert(0, 'Reforge.Result')
    $out
}

# ============================================================================
#  Private helpers: GitHub
# ============================================================================

function ConvertTo-RepositoryName([string]$Repository) {
    # owner/repo from owner/repo or a GitHub link; nothing if it's neither
    $repo = $Repository.Trim() -replace '^https?://(www\.)?github\.com/', '' -replace '\.git$', '' -replace '/+$', ''
    if ($repo -match '^[\w.-]+/[\w.-]+$') { $repo }
}

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
    $path = Join-Path $script:WorkRoot ('reforge-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
    New-Item -ItemType Directory -Path $path -WhatIf:$false | Out-Null
    $path
}

function Remove-StaleWorkFolder {
    # Work folders are deleted after each update; this catches ones left by a crashed or killed run
    Get-ChildItem -LiteralPath $script:WorkRoot -Directory -Filter 'reforge-*' -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^reforge-[0-9a-f]{8}$' -and $_.LastWriteTime -lt (Get-Date).AddDays(-1) } |
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue -WhatIf:$false
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
    # With nobody to answer (pwsh -NonInteractive, a scheduled task, a service), use the default.
    # Every prompt in this module defaults to the safe answer.
    $choices = [System.Management.Automation.Host.ChoiceDescription[]]@($Labels | ForEach-Object { [System.Management.Automation.Host.ChoiceDescription]::new($_) })
    try {
        if (-not [Environment]::UserInteractive) { throw [InvalidOperationException]::new('No interactive session.') }
        $Host.UI.PromptForChoice($Caption, $Message, $choices, $Default)
    } catch [InvalidOperationException], [System.Management.Automation.Host.HostException], [IO.IOException] {
        Write-Warning "Can't ask here: $Message Answering '$($Labels[$Default] -replace '&')'."
        $Default
    }
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
        @{ Mine = '(?i)arm64|aarch64'; Other = '(?i)x64|amd64|x86_64|win64|64-?bit|ia32|i386|(?<!_)x86' }
    } else {
        @{ Mine = '(?i)x64|amd64|x86_64|win64|64-?bit'; Other = '(?i)arm64|aarch64|ia32|i386|(?<!_)x86(?!_64)|32-?bit' }
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
        Where-Object {
            $_.name -match '(?i)\.(msi|exe)$' -and
            $_.name -notmatch '(?i)portable|\.(7z|zip)\.exe$' -and   # portable builds and self-extracting archives aren't installers
            -not (Test-NameHasWord $_.name 'mac', 'macos', 'osx', 'darwin', 'linux')
        } |
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

function Get-InstallerSilentArg([string]$File) {
    # Detect common installer types from the first few MB of the file
    $fs = [IO.File]::OpenRead($File)
    try {
        $buf = New-Object byte[] ([Math]::Min(4MB, $fs.Length))
        [void]$fs.Read($buf, 0, $buf.Length)
    } finally { $fs.Dispose() }
    $text = [Text.Encoding]::ASCII.GetString($buf)
    if     ($text -match 'Inno Setup') { '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART' }
    elseif ($text -match 'Nullsoft')   { '/S' }
}

function Start-InstallerProcess([string]$FilePath, [string]$ArgumentList) {
    # Waits for the installer only. Start-Process -Wait also waits for everything it starts, so an
    # installer that opens the app when it's done would block until you close the app.
    $params = @{ FilePath = $FilePath; PassThru = $true }
    if ($ArgumentList) { $params.ArgumentList = $ArgumentList }
    $p = Start-Process @params
    $null = $p.Handle   # without an open handle, Windows PowerShell can lose the exit code
    $p.WaitForExit()
    $p.ExitCode
}

function Invoke-Installer([string]$File, [string]$SilentArgs) {
    if ($File -match '(?i)\.msi$') { return Start-InstallerProcess msiexec.exe "/i `"$File`" /qn /norestart" }
    if (-not $SilentArgs) { $SilentArgs = Get-InstallerSilentArg $File }
    if (-not $SilentArgs) { Write-Warning "Unknown installer type for $([IO.Path]::GetFileName($File)); opening it normally." }
    Start-InstallerProcess $File $SilentArgs
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
    if (-not (Test-PerUserInstall $Program) -and -not (Test-IsAdmin)) { $Result.Status = 'NeedsAdmin'; $Result.Note = 'Installed for all users; run PowerShell as administrator'; return }
    if (-not (Approve-Install $App $asset)) { $Result.Status = 'Skipped'; $Result.Note = 'Unverified download declined'; return }
    Install-ReleaseAsset $App $asset $Result 'Updated'
}

function Install-ReleaseAsset($App, $Asset, $Result, [string]$DoneStatus) {
    $work = New-WorkFolder
    try {
        $file = Save-ReleaseAsset $Asset $work
        Write-Verbose "Installing $($Asset.name)"
        $code = Invoke-Installer $file $App.SilentArgs
    } finally {
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue -WhatIf:$false
    }
    if ($code -in 0, 3010, 1641) {
        $Result.Status = $DoneStatus
        $Result.Note = if ($code -ne 0) { 'Restart needed to finish' } elseif ($Asset.digest) { 'Checksum verified' } else { 'Unverified' }
    } else {
        $Result.Status = 'Failed'
        $Result.Note = "Installer exit code $code"
    }
}

function Get-ProgramKey($Program) { "$($Program.PSPath)|$($Program.DisplayVersion)" }

function Invoke-InstallerAppInstall($App, $Release, $Result, $Cmdlet, [bool]$Force) {
    $asset = Select-InstallerAsset $Release $App.Pattern $true   # .msi when there's a choice
    if (-not $asset) { $Result.Status = 'Skipped'; $Result.Note = 'No Windows installer in this release (for a zip tool, use Add-ReforgeApp -Destination)'; return }
    if ($asset.digest) { Set-AppField $App 'HadDigest' $true }

    $Result.Status = 'Available'
    $Result.Note = Get-VerificationNote $App $asset
    if (-not $Cmdlet.ShouldProcess("$($App.Repository) $($Release.tag_name)", "Install $($asset.name)")) { return }
    # Installing something new always gets a question; the unverified-download prompt is that question
    $approved = if (-not $asset.digest -and -not $App.AllowUnverified) { Approve-Install $App $asset }
                elseif ($Force) { $true }
                else { (Read-Choice 'Install' "Install $($asset.name) from $($App.Repository)? ($($Result.Note))" @('&Yes', '&No') 1) -eq 0 }
    if (-not $approved) { $Result.Status = 'Skipped'; $Result.Note = 'Not installed'; return }

    # What the installer added (or changed, if the app was already there) tells us its name in Installed apps
    $before = @(Get-InstalledProgram | ForEach-Object { Get-ProgramKey $_ })
    Install-ReleaseAsset $App $asset $Result 'Installed'
    if ($Result.Status -ne 'Installed') {
        if (-not (Test-IsAdmin)) { $Result.Note += '. It may need PowerShell running as administrator' }
        return
    }
    $leaf = $App.Repository.Split('/')[1]
    $program = Get-InstalledProgram | Where-Object { (Get-ProgramKey $_) -notin $before } |
               Sort-Object { $_.DisplayName -notlike "*$leaf*" } | Select-Object -First 1
    if ($program) {
        if (-not $App.Name) { $App.Name = Get-ProgramBaseName $program.DisplayName }
        $Result.Installed = $program.DisplayVersion
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

function Remove-OldBackup([string]$AppBackupDir, [int]$Keep) {
    # Backup folders are named by date and time, so sorting by name puts the newest first
    Get-ChildItem -LiteralPath $AppBackupDir -Directory -ErrorAction SilentlyContinue |
        Where-Object Name -match '^\d{4}-\d\d-\d\d_\d\d-\d\d-\d\d$' |
        Sort-Object Name -Descending |
        Select-Object -Skip $Keep |
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue -WhatIf:$false
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
        $appBackups = Join-Path (Join-Path (Split-Path -Parent $ConfigPath) 'backups') $safeName
        $backupRoot = Join-Path $appBackups $stamp
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
        Remove-OldBackup $appBackups $script:BackupsToKeep

        Set-AppField $App 'InstalledVersion' $Release.tag_name
        $Result.Installed = $Release.tag_name
        $Result.Status = 'Updated'
        $Result.Note = if ($asset.digest) { 'Checksum verified' } else { 'Unverified' }
    } finally {
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue -WhatIf:$false
    }
}

# ============================================================================
#  Private helpers: finding apps from GitHub
# ============================================================================

function Get-DownloadsFolder {
    # The Downloads folder can be moved, so ask Windows where it is
    $folders = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders' -ErrorAction Ignore
    $known = if ($folders) { $folders.'{374DE290-123F-4565-9164-39C4925E467B}' }
    if ($known) { $known } else { Join-Path $env:USERPROFILE 'Downloads' }
}

function Get-DownloadOrigin([string]$File) {
    # Browsers note where a file came from in its Zone.Identifier stream (the "Mark of the Web").
    # GitHub serves release files from another host, so the repository and tag come from the
    # release page it was downloaded from (ReferrerUrl).
    $note = Get-Content -LiteralPath $File -Stream Zone.Identifier -ErrorAction Ignore
    if (($note -join "`n") -match 'https://github\.com/([\w.-]+/[\w.-]+)/releases/(?:tag|download)/([^/?#\s]+)') {
        [pscustomobject]@{ Repository = $Matches[1]; Tag = [uri]::UnescapeDataString($Matches[2]) }
    }
}

function Get-GitHubDownload([string[]]$Path) {
    # Two levels of subfolders, for downloads sorted into folders like Downloads\Mods.
    # Newest first, so each repository's latest download wins.
    Get-ChildItem -LiteralPath $Path -File -Recurse -Depth 2 |
        Where-Object Name -match '(?i)\.(msi|exe|zip|7z|tar\.gz|tgz|tar\.xz)$' |
        Sort-Object LastWriteTime -Descending |
        ForEach-Object {
            $origin = Get-DownloadOrigin $_.FullName
            if ($origin) { $origin | Add-Member -NotePropertyName File -NotePropertyValue $_.Name -PassThru }
        }
}

function Get-VersionText([string]$Text) {
    # "v1.2.0" and "1.2.0.0" both become "1.2", so versions written differently compare equal
    $v = ConvertTo-AppVersion $Text
    if ($v) { "$v" -replace '(\.0)+$' }
}

function Find-DownloadedProgram($Programs, $Download) {
    # The installed app with the release's version whose name (its first word, like "Notepad++")
    # is in the file or repository name. Both must match, so a download alone never suggests an app.
    $version = Get-VersionText $Download.Tag
    if (-not $version) { return }
    $names = ($Download.File + $Download.Repository) -replace '[^a-zA-Z0-9]'
    $Programs | Where-Object {
        $word = ($_.DisplayName -split '\s')[0] -replace '[^a-zA-Z0-9]'
        $word.Length -ge 3 -and $names -like "*$word*" -and (Get-VersionText $_.DisplayVersion) -eq $version
    } | Select-Object -First 1
}

function Test-WingetApp($Program, $WingetPackages) {
    # winget shortens names ("PowerShell 7-x64" is "PowerShell"), so match the start of the name and the version
    [bool]($WingetPackages | Where-Object { $_.Name -and $Program.DisplayName.StartsWith($_.Name, [StringComparison]::OrdinalIgnoreCase) -and $_.InstalledVersion -eq $Program.DisplayVersion })
}

function New-Candidate($Program, [string]$Repository, [string]$Version) {
    # An installed app, or an archive tool known only from its download (no $Program)
    [pscustomobject]@{
        PSTypeName = 'Reforge.Candidate'
        Name       = if ($Program) { Get-ProgramBaseName $Program.DisplayName } else { $Repository.Split('/')[1] }   # no version, so it stays valid after updates
        Repository = $Repository
        Version    = if ($Program) { $Program.DisplayVersion } else { $Version }
        Type       = if ($Program) { 'Installer' } else { 'Archive' }
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

        # From Find-ReforgeApp: an archive tool can't be tracked without knowing its folder
        [Parameter(ValueFromPipelineByPropertyName, DontShow)]
        [string]$Type,

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
        $repo = ConvertTo-RepositoryName $Repository
        if (-not $repo) {
            Write-Error "'$Repository' isn't a GitHub repository. Use owner/repo, for example storytold/photocraft."
            return
        }
        $appName = if ($Name) { $Name } else { $repo.Split('/')[1] }
        if ($Type -eq 'Archive' -and -not $Destination) {
            Write-Error "'$appName' is an archive tool. Add it with -Destination, the folder its files go into."
            return
        }

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

function Install-ReforgeApp {
    [CmdletBinding(SupportsShouldProcess)]
    [OutputType('Reforge.Result')]
    param(
        [Parameter(Mandatory, Position = 0, ValueFromPipelineByPropertyName)]
        [string]$Repository,

        [Parameter(Position = 1, ValueFromPipelineByPropertyName)]
        [string]$Name,

        [Parameter(ValueFromPipelineByPropertyName)]
        [string]$Pattern,

        [Parameter(ValueFromPipelineByPropertyName)]
        [string]$SilentArgs,

        [Parameter(ValueFromPipelineByPropertyName)]
        [switch]$Prerelease,

        [Parameter(ValueFromPipelineByPropertyName)]
        [switch]$AllowUnverified,

        [securestring]$Token,

        [switch]$Force,

        [ValidateNotNullOrEmpty()][string]$ConfigPath = $script:DefaultConfigPath
    )
    begin {
        $apps = [System.Collections.Generic.List[object]]::new()
        foreach ($a in Read-AppList $ConfigPath) { $apps.Add($a) }
        $headers = Get-RequestHeader $Token
        if (-not $WhatIfPreference) { Remove-StaleWorkFolder }
    }
    process {
        $repo = ConvertTo-RepositoryName $Repository
        if (-not $repo) {
            Write-Error "'$Repository' isn't a GitHub repository. Use owner/repo, for example storytold/photocraft."
            return
        }
        $tracked = @($apps | Where-Object { ($_.Repository -eq $repo -and $_.Type -ne 'archive') -or ($Name -and $_.Name -eq $Name) })
        if ($tracked) {
            Write-Error "'$($tracked[0].Name)' is already in the list. Update it with Update-ReforgeApp."
            return
        }

        $entry = [ordered]@{ Name = $Name; Repository = $repo }
        if ($Pattern)         { $entry.Pattern = $Pattern }
        if ($SilentArgs)      { $entry.SilentArgs = $SilentArgs }
        if ($Prerelease)      { $entry.Prerelease = $true }
        if ($AllowUnverified) { $entry.AllowUnverified = $true }
        $app = [pscustomobject]$entry
        $result = New-Result $app
        try {
            $release = Get-LatestRelease $repo ([bool]$Prerelease) $headers
            if ($release) {
                $result.Latest = $release.tag_name
                Invoke-InstallerAppInstall $app $release $result $PSCmdlet ([bool]$Force)
            } else {
                $result.Status = 'Skipped'
                $result.Note = 'No releases found'
            }
        } catch {
            $result.Status = 'Failed'
            $result.Note = $_.Exception.Message
            Write-Warning "${repo}: $($_.Exception.Message)"
        }

        $nameFound = [bool]$app.Name
        if (-not $nameFound) { $app.Name = $repo.Split('/')[1] }
        $result.Name = $app.Name
        if ($result.Status -eq 'Installed') {
            # Tracking it under a guessed name would only make every update say NotInstalled
            if (-not $nameFound) {
                $result.Note += '. Not tracked'
                Write-Warning "Installed, but it didn't show up as a new app in Installed apps (was it already installed?), so it isn't tracked. Track it with the name Settings shows: Add-ReforgeApp $repo -Name '<name>'"
            } elseif ($apps | Where-Object Name -eq $app.Name) {
                Write-Warning "'$($app.Name)' is already in the list, so this app isn't tracked. Track it with Add-ReforgeApp $repo -Name '<another name>'."
            } else {
                $apps.Add($app)
                Save-AppList $ConfigPath $apps
            }
        }
        ConvertTo-ResultView $result
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
        [Parameter(Position = 0)]
        [string[]]$Path = (Get-DownloadsFolder),

        [ValidateNotNullOrEmpty()][string]$ConfigPath = $script:DefaultConfigPath
    )
    $tracked = @(Read-AppList $ConfigPath | ForEach-Object { $_.Repository })
    $winget = @()
    if (Get-Command -Name Get-WinGetPackage -ErrorAction SilentlyContinue) {
        # Skip apps winget already updates, so they aren't updated twice
        $winget = @(Get-WinGetPackage | Where-Object Source -eq 'winget')
    }
    $seen = @{}
    $programs = @(Get-InstalledProgram)

    # Installed apps that link to GitHub in their uninstall information
    foreach ($program in $programs) {
        $urls = @($program.URLInfoAbout, $program.URLUpdateInfo, $program.HelpLink) -join ' '
        if ($urls -notmatch 'github\.com/([\w.-]+)/([\w.-]+)') { continue }
        $owner = $Matches[1]
        $repo  = "$owner/$($Matches[2] -replace '\.git$', '')"
        if ($owner -in 'sponsors', 'orgs', 'apps', 'features', 'settings') { continue }
        if ($tracked -contains $repo -or $seen[$repo] -or (Test-WingetApp $program $winget)) { continue }
        $seen[$repo] = $true
        New-Candidate $program $repo
    }

    # Files downloaded from GitHub releases: archive tools, and installers of apps that are installed
    foreach ($download in Get-GitHubDownload $Path) {
        $repo = $download.Repository
        if ($tracked -contains $repo -or $seen[$repo]) { continue }
        if ($download.File -match '(?i)\.(zip|7z|tar\.gz|tgz|tar\.xz)$') {
            $seen[$repo] = $true
            New-Candidate $null $repo $download.Tag
            continue
        }
        $program = Find-DownloadedProgram $programs $download
        if (-not $program -or (Test-WingetApp $program $winget)) { continue }
        $seen[$repo] = $true
        New-Candidate $program $repo
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

        if (-not $WhatIfPreference) { Remove-StaleWorkFolder }
        $headers = Get-RequestHeader $Token
        $programs = $null
        $script:ListChanged = $false
        $i = 0

        foreach ($app in $targets) {
            $i++
            Write-Progress -Activity 'Updating Reforge apps' -Status $app.Name -PercentComplete (($i - 1) / $targets.Count * 100)
            $result = New-Result $app
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
            ConvertTo-ResultView $result
        }
        Write-Progress -Activity 'Updating Reforge apps' -Completed
    }
}

Export-ModuleMember -Function Get-ReforgeApp, Add-ReforgeApp, Install-ReforgeApp, Remove-ReforgeApp, Find-ReforgeApp, Update-ReforgeApp
