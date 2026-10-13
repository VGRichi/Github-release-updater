<#
.SYNOPSIS
Runs Reforge's Pester tests.

.DESCRIPTION
Runs everything in tests/. Tests tagged KnownBug describe behaviour that's wrong today; they're
left out of the normal run so it stays green. Use -KnownBugs to run only those and see what's
still open. When you fix one, remove its KnownBug tag.

Uses Pester 5, installed for the current user if missing (no admin needed).

.EXAMPLE
.\build\invoke-tests.ps1
Runs the tests.

.EXAMPLE
.\build\invoke-tests.ps1 -KnownBugs
Runs only the tests for known bugs (these are expected to fail).

.EXAMPLE
.\build\invoke-tests.ps1 -CodeCoverage
Also measures which lines of Reforge.psm1 the tests run (out/coverage.xml).
#>
[CmdletBinding()]
param(
    [switch]$KnownBugs,
    [switch]$CodeCoverage,
    [switch]$CI
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$out  = Join-Path $root 'out'
$minPester = [version]'5.5.0'

if (-not (Get-Module Pester -ListAvailable | Where-Object { $_.Version -ge $minPester })) {
    Write-Host "Installing Pester 5 for the current user..."
    # -SkipPublisherCheck: Windows ships Pester 3.4, signed by a different publisher
    Install-Module Pester -MinimumVersion $minPester -Scope CurrentUser -Force -SkipPublisherCheck
}
Import-Module Pester -MinimumVersion $minPester

$config = New-PesterConfiguration
$config.Run.Path = Join-Path $root 'tests'
$config.Run.Exit = [bool]$CI
$config.Output.Verbosity = 'Detailed'
if ($KnownBugs) { $config.Filter.Tag = 'KnownBug' } else { $config.Filter.ExcludeTag = 'KnownBug' }

if ($CodeCoverage) {
    $config.CodeCoverage.Enabled = $true
    $config.CodeCoverage.Path = Join-Path $root 'Reforge\Reforge.psm1'
    $config.CodeCoverage.OutputPath = Join-Path $out 'coverage.xml'
}
if ($CI) {
    $config.TestResult.Enabled = $true
    $config.TestResult.OutputPath = Join-Path $out "test-results-$($PSVersionTable.PSEdition).xml"
}

Invoke-Pester -Configuration $config
