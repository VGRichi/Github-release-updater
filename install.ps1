# Installs Update-GitHubApps and adds the `upgh` command to your PowerShell profile.
# Usage:  irm https://raw.githubusercontent.com/YOUR-USERNAME/github-app-updater/main/install.ps1 | iex

$ErrorActionPreference = 'Stop'
$ProgressPreference    = 'SilentlyContinue'
$Repo   = 'VGRichi/github-app-updater'
$Branch = 'main'

$scriptsDir = Join-Path (Split-Path $PROFILE) 'Scripts'
New-Item $scriptsDir -ItemType Directory -Force | Out-Null
$dest = Join-Path $scriptsDir 'Update-GitHubApps.ps1'

Write-Host 'Downloading Update-GitHubApps.ps1...' -ForegroundColor Cyan
Invoke-WebRequest "https://raw.githubusercontent.com/$Repo/$Branch/Update-GitHubApps.ps1" -OutFile $dest -UseBasicParsing
Unblock-File $dest

if (-not (Test-Path $PROFILE)) { New-Item $PROFILE -ItemType File -Force | Out-Null }
$line = 'function upgh { & "$(Split-Path $PROFILE)\Scripts\Update-GitHubApps.ps1" @args }'
if (-not (Select-String -Path $PROFILE -SimpleMatch 'Update-GitHubApps.ps1' -Quiet)) {
    Add-Content $PROFILE "`n$line"
    Write-Host 'Added the "upgh" command to your profile.' -ForegroundColor Green
} else {
    Write-Host 'The "upgh" command is already in your profile (script updated).' -ForegroundColor Green
}

if ((Get-ExecutionPolicy) -in 'Restricted', 'AllSigned', 'Undefined') {
    Write-Host ''
    Write-Host 'Your PowerShell blocks profile scripts. Run this once to allow them:' -ForegroundColor Yellow
    Write-Host '  Set-ExecutionPolicy RemoteSigned -Scope CurrentUser'
}

Write-Host ''
Write-Host 'Done! Open a new PowerShell window, then try:' -ForegroundColor Cyan
Write-Host '  upgh -Discover        find GitHub apps you have installed'
Write-Host '  upgh -Add owner/repo -Name "App name"'
Write-Host '  upgh                 update everything'
