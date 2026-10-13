# Installs Update-GitHubApps from the latest release and adds the `upgh` command to your PowerShell profile.
# Usage:  irm https://github.com/VGRichi/github-app-updater/releases/latest/download/install.ps1 | iex

$ErrorActionPreference = 'Stop'
$ProgressPreference    = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$Repo    = 'VGRichi/github-app-updater'
$Headers = @{ 'User-Agent' = 'github-app-updater-installer'; 'Accept' = 'application/vnd.github+json' }

Write-Host 'Finding the latest release...' -ForegroundColor Cyan
$release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -Headers $Headers
$asset   = $release.assets | Where-Object name -eq 'Update-GitHubApps.ps1' | Select-Object -First 1
if (-not $asset) { throw "Release $($release.tag_name) has no Update-GitHubApps.ps1 attached." }

$scriptsDir = Join-Path (Split-Path $PROFILE) 'Scripts'
New-Item $scriptsDir -ItemType Directory -Force | Out-Null
$dest = Join-Path $scriptsDir 'Update-GitHubApps.ps1'
$tmp  = Join-Path $env:TEMP "Update-GitHubApps-$($release.tag_name).ps1"

Write-Host "Downloading version $($release.tag_name)..." -ForegroundColor Cyan
Invoke-WebRequest $asset.browser_download_url -OutFile $tmp -Headers $Headers -UseBasicParsing

if ($asset.digest) {
    $hash = (Get-FileHash $tmp -Algorithm SHA256).Hash
    if ("sha256:$hash" -ne $asset.digest) {
        Remove-Item $tmp -Force
        throw 'Downloaded file does not match the release checksum. Nothing was installed.'
    }
    Write-Host 'Checksum verified.'
}

Move-Item $tmp $dest -Force
Unblock-File $dest

if (-not (Test-Path $PROFILE)) { New-Item $PROFILE -ItemType File -Force | Out-Null }
$line = 'Set-Alias upgh "$(Split-Path $PROFILE)\Scripts\Update-GitHubApps.ps1"'
if (-not (Select-String -Path $PROFILE -SimpleMatch 'Update-GitHubApps.ps1' -Quiet)) {
    Add-Content $PROFILE "`n$line"
    Write-Host 'Added the "upgh" command to your profile.' -ForegroundColor Green
} else {
    Write-Host 'The "upgh" command is already in your profile.' -ForegroundColor Green
}

if ((Get-ExecutionPolicy) -in 'Restricted', 'AllSigned', 'Undefined') {
    Write-Host ''
    Write-Host 'Your PowerShell blocks profile scripts. Run this once to allow them:' -ForegroundColor Yellow
    Write-Host '  Set-ExecutionPolicy RemoteSigned -Scope CurrentUser'
}

Write-Host ''
Write-Host "Installed GitHub App Updater $($release.tag_name). Open a new PowerShell window, then try:" -ForegroundColor Cyan
Write-Host '  upgh -Discover        find GitHub apps you have installed'
Write-Host '  upgh -Add owner/repo -Name "App name"'
Write-Host '  upgh                 update everything'
Write-Host ''
Write-Host 'To update this tool later, run the install command again.' -ForegroundColor DarkGray