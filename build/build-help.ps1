<#
.SYNOPSIS
Builds Reforge's help from the Markdown files in docs/.

.DESCRIPTION
docs/<culture>/*.md (for example docs/en-US) is the source of the module's help.
This script turns each culture folder into the XML help that Get-Help reads
(Reforge/<culture>/Reforge-help.xml).

-Init is a one-time step: it creates docs/en-US from the comment-based help in the
module, then removes those comment blocks from the code.

-Cab also builds the files Update-Help downloads (a .cab/.zip per culture and a
HelpInfo.xml) into out/help. Building .cab files needs Windows.

Uses platyPS 0.14.2, installed for the current user if missing (no admin needed).

.EXAMPLE
.\build\Build-Help.ps1 -Init
One-time: create docs/en-US from the current help and build the XML.

.EXAMPLE
.\build\Build-Help.ps1
Rebuild the XML help after editing files in docs/.
#>
[CmdletBinding()]
param(
    [switch]$Init,
    [switch]$Cab,
    [string]$OutputPath = (Join-Path (Split-Path -Parent $PSScriptRoot) 'out')
)

$ErrorActionPreference = 'Stop'
$root      = Split-Path -Parent $PSScriptRoot
$moduleDir = Join-Path $root 'Reforge'
$docsRoot  = Join-Path $root 'docs'
$manifest  = Import-PowerShellDataFile (Join-Path $moduleDir 'Reforge.psd1')
$version   = $manifest.ModuleVersion
$repoUrl   = $manifest.PrivateData.PSData.ProjectUri
$platyVer  = '0.14.2'

if (-not (Get-Module platyPS -ListAvailable | Where-Object { $_.Version -eq $platyVer })) {
    Write-Host "Installing platyPS $platyVer for the current user..."
    Install-Module platyPS -RequiredVersion $platyVer -Scope CurrentUser -Force -AllowClobber
}
Import-Module platyPS -RequiredVersion $platyVer -Force

# ---------------------------------------------------------------- one-time conversion
if ($Init) {
    $enUS = Join-Path $docsRoot 'en-US'
    if (Test-Path $enUS) { throw 'docs/en-US already exists. -Init is a one-time step; edit the files in docs/ instead.' }

    $psm1 = Join-Path $moduleDir 'Reforge.psm1'
    $code = Get-Content $psm1 -Raw
    $helpBlock = '(?ms)^[ \t]*<#[ \t]*\r?\n[ \t]*\.SYNOPSIS.*?#>[ \t]*\r?\n'
    $blocks = [regex]::Matches($code, $helpBlock).Count
    if ($blocks -eq 0) { throw 'No comment-based help found in the module. Has -Init already been run?' }

    Write-Host 'Creating docs/en-US from the comment-based help...'
    Import-Module $moduleDir -Force
    New-MarkdownHelp -Module Reforge -OutputFolder $enUS -WithModulePage -AlphabeticParamsOrder `
        -Locale 'en-US' -HelpVersion $version -FwLink $manifest.HelpInfoURI -Force | Out-Null

    foreach ($file in Get-ChildItem $enUS -Filter '*.md') {
        $text = Get-Content $file.FullName -Raw
        if ($file.BaseName -eq 'Reforge') {
            $text = $text -replace '\{\{ ?Fill in the Description ?\}\}', $manifest.Description
        } else {
            # Lets Get-Help -Online open the page on GitHub
            $text = $text -replace '(?m)^online version:.*$', "online version: $repoUrl/blob/main/docs/en-US/$($file.Name)"
        }
        # platyPS 0.14 lists PowerShell 7.4's -ProgressAction common parameter as a normal one; remove it
        $text = $text -replace ' \[-ProgressAction <ActionPreference>\]', ''
        $text = $text -replace '(?s)### -ProgressAction.*?(?=### |## )', ''
        Set-Content -LiteralPath $file.FullName -Value $text -Encoding utf8 -NoNewline
    }

    Write-Host "Removing $blocks comment-based help blocks from Reforge.psm1..."
    Set-Content -LiteralPath $psm1 -Value ([regex]::Replace($code, $helpBlock, '')) -Encoding utf8 -NoNewline
}

# ---------------------------------------------------------------- build XML help
$cultures = @(Get-ChildItem $docsRoot -Directory -ErrorAction SilentlyContinue)
if (-not $cultures) { throw 'No docs/<culture> folders found. Run with -Init first.' }

foreach ($culture in $cultures) {
    $out = Join-Path $moduleDir $culture.Name
    New-ExternalHelp -Path $culture.FullName -OutputPath $out -Force | Out-Null
    Write-Host "Built $($culture.Name) help -> Reforge/$($culture.Name)"

    $placeholders = Select-String -Path (Join-Path $culture.FullName '*.md') -Pattern '\{\{.*?\}\}'
    foreach ($p in $placeholders) {
        Write-Warning "Unfilled placeholder in docs/$($culture.Name)/$($p.Filename) line $($p.LineNumber): $($p.Matches[0].Value)"
    }
}

# ---------------------------------------------------------------- Update-Help files
if ($Cab) {
    $helpOut = Join-Path $OutputPath 'help'
    if (Test-Path $helpOut) { Remove-Item $helpOut -Recurse -Force }
    New-Item -ItemType Directory -Path $helpOut -Force | Out-Null

    $v = [version]$version
    $helpVersion = '{0}.{1}.{2}.{3}' -f $v.Major, $v.Minor, [Math]::Max($v.Build, 0), [Math]::Max($v.Revision, 0)
    $temp = Join-Path ([IO.Path]::GetTempPath()) ('gau-help-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
    New-Item -ItemType Directory -Path $temp | Out-Null
    try {
        foreach ($culture in $cultures) {
            # Stamp the landing page with this release's help version (on a copy, not the source)
            $landing = Join-Path $temp "Reforge.$($culture.Name).md"
            $text = Get-Content (Join-Path $culture.FullName 'Reforge.md') -Raw
            $text = $text -replace '(?m)^Help Version:.*$', "Help Version: $helpVersion"
            $text = $text -replace '(?m)^Download Help Link:.*$', "Download Help Link: $($manifest.HelpInfoURI)"
            Set-Content -LiteralPath $landing -Value $text -Encoding utf8 -NoNewline

            New-ExternalHelpCab -CabFilesFolder (Join-Path $moduleDir $culture.Name) -LandingPagePath $landing -OutputFolder $helpOut | Out-Null
            Write-Host "Built Update-Help files for $($culture.Name) -> out/help"
        }
    } finally {
        Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue
    }
}
