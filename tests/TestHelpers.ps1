# Shared helpers for the Reforge tests. Dot-source this from a BeforeAll block.
#
# Private module functions are called with InModuleScope. Objects built here are passed in
# with -Parameters, because InModuleScope can't see the test's own variables or functions.
#
# Tests tagged KnownBug describe the behaviour we want but don't have yet. They're excluded
# from the normal run (see build/invoke-tests.ps1); fixing a bug means removing its tag.

$ModuleRoot = Join-Path (Split-Path -Parent $PSScriptRoot) 'Reforge'

function Import-ReforgeForTest {
    # Point APPDATA at the test drive, so importing the module (which migrates old app lists)
    # never touches the real %APPDATA%\Reforge
    $env:APPDATA = Join-Path $TestDrive 'AppData'
    New-Item -ItemType Directory -Path $env:APPDATA -Force | Out-Null
    Import-Module $ModuleRoot -Force
}

function New-TestConfigPath {
    Join-Path $TestDrive ('apps-' + [guid]::NewGuid().ToString('N').Substring(0, 8) + '.json')
}

function Write-TestAppList([string]$Path, [object[]]$Apps) {
    ConvertTo-Json -InputObject @($Apps) -Depth 5 | Set-Content -LiteralPath $Path -Encoding utf8
}

function Read-TestAppList([string]$Path) {
    # foreach unrolls the array; Windows PowerShell's ConvertFrom-Json returns it as one object
    foreach ($item in (ConvertFrom-Json -InputObject (Get-Content -LiteralPath $Path -Raw))) { $item }
}

function New-TestAsset([string]$Name, [string]$Digest) {
    [pscustomobject]@{
        name                 = $Name
        browser_download_url = "https://example.test/download/$Name"
        digest               = $Digest
    }
}

function New-TestRelease([string]$Tag = 'v1.1.0', [object[]]$Assets = @(), [switch]$Draft, [switch]$Prerelease) {
    [pscustomobject]@{
        tag_name   = $Tag
        draft      = [bool]$Draft
        prerelease = [bool]$Prerelease
        assets     = @($Assets)
    }
}

function New-TestProgram([string]$Name, [string]$Version, [switch]$AllUsers, [switch]$Msi) {
    $hive = if ($AllUsers) { 'HKEY_LOCAL_MACHINE' } else { 'HKEY_CURRENT_USER' }
    [pscustomobject]@{
        DisplayName      = $Name
        DisplayVersion   = $Version
        PSPath           = "Microsoft.PowerShell.Core\Registry::$hive\Software\Microsoft\Windows\CurrentVersion\Uninstall\$Name"
        WindowsInstaller = if ($Msi) { 1 } else { 0 }
    }
}

function Get-TestDigest([byte[]]$Bytes) {
    # Same format as the "digest" field GitHub returns for release assets
    $sha = [Security.Cryptography.SHA256]::Create()
    try { 'sha256:' + (($sha.ComputeHash($Bytes) | ForEach-Object { $_.ToString('x2') }) -join '') }
    finally { $sha.Dispose() }
}

function New-HttpError([int]$StatusCode) {
    # Looks enough like Invoke-RestMethod's error for $_.Exception.Response.StatusCode
    $ex = [Exception]::new("Response status code does not indicate success: $StatusCode")
    $ex | Add-Member -NotePropertyName Response -NotePropertyValue ([pscustomobject]@{ StatusCode = $StatusCode })
    $ex
}

function New-TestTree([string]$Root, [hashtable]$Files) {
    # $Files maps relative paths to file contents
    foreach ($rel in $Files.Keys) {
        $path = Join-Path $Root $rel
        New-Item -ItemType Directory -Path (Split-Path -Parent $path) -Force | Out-Null
        Set-Content -LiteralPath $path -Value $Files[$rel] -NoNewline
    }
    $Root
}
