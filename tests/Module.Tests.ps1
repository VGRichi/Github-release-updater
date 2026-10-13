Describe 'The Reforge module' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        $repoRoot = Split-Path -Parent $PSScriptRoot
        $manifestPath = Join-Path $ModuleRoot 'Reforge.psd1'
        $manifest = Import-PowerShellDataFile $manifestPath
        Import-ReforgeForTest
        $exported = @((Get-Module Reforge).ExportedFunctions.Keys)
    }
    AfterAll {
        $env:APPDATA = $originalAppData
        # Don't re-import here: with the real APPDATA, importing could migrate your real app list
        Remove-Module Reforge -ErrorAction SilentlyContinue
    }

    Context 'Manifest and exports' {
        It 'has a valid manifest' {
            { Test-ModuleManifest -Path $manifestPath -ErrorAction Stop } | Should -Not -Throw
        }

        It 'exports exactly the functions the manifest lists' {
            $exported | Sort-Object | Should -Be ($manifest.FunctionsToExport | Sort-Object)
        }

        It 'exports no private helpers' {
            $exported | Where-Object { $_ -notlike '*-Reforge*' } | Should -BeNullOrEmpty
        }

        It 'has a table view for <TypeName>' -TestCases @(
            @{ TypeName = 'Reforge.App' }, @{ TypeName = 'Reforge.Result' }, @{ TypeName = 'Reforge.Candidate' }
        ) {
            Get-FormatData -TypeName $TypeName | Should -Not -BeNullOrEmpty
        }
    }

    Context 'Help' {
        It '<Command> has a synopsis and examples' -TestCases @(
            @{ Command = 'Add-ReforgeApp' }, @{ Command = 'Get-ReforgeApp' }, @{ Command = 'Remove-ReforgeApp' }
            @{ Command = 'Find-ReforgeApp' }, @{ Command = 'Update-ReforgeApp' }
        ) {
            $help = Get-Help $Command -Full
            $help.Synopsis | Should -Not -BeLike "*$Command*"   # Get-Help falls back to the syntax when there is no help
            @($help.Examples.Example).Count | Should -BeGreaterThan 0
        }

        It '<Command> has a page in docs/en-US' -TestCases @(
            @{ Command = 'Add-ReforgeApp' }, @{ Command = 'Get-ReforgeApp' }, @{ Command = 'Remove-ReforgeApp' }
            @{ Command = 'Find-ReforgeApp' }, @{ Command = 'Update-ReforgeApp' }
        ) {
            Join-Path $repoRoot "docs\en-US\$Command.md" | Should -Exist
        }

        It 'describes each command with its own synopsis on the module help page' -Tag KnownBug {
            $page = Get-Content -LiteralPath (Join-Path $repoRoot 'docs\en-US\Reforge.md') -Raw
            ([regex]::Matches($page, [regex]::Escape($manifest.Description))).Count | Should -BeLessOrEqual 1
        }
    }

    Context 'Importing' {
        It 'creates nothing in APPDATA when there is no old app list' {
            $env:APPDATA = Join-Path $TestDrive 'EmptyAppData'
            New-Item -ItemType Directory -Path $env:APPDATA | Out-Null
            Import-Module $ModuleRoot -Force
            Get-ChildItem -LiteralPath $env:APPDATA | Should -BeNullOrEmpty
        }

        It 'copies a GitHubAppUpdater app list to the new location' {
            $env:APPDATA = Join-Path $TestDrive 'LegacyAppData'
            $legacy = Join-Path $env:APPDATA 'GitHubAppUpdater\github-apps.json'
            New-Item -ItemType Directory -Path (Split-Path -Parent $legacy) -Force | Out-Null
            Write-TestAppList $legacy @([pscustomobject]@{ Name = 'Old'; Repo = 'owner/old' })
            Import-Module $ModuleRoot -Force
            Join-Path $env:APPDATA 'Reforge\apps.json' | Should -Exist
            (Get-ReforgeApp).Repository | Should -Be 'owner/old'
            Test-Path -LiteralPath $legacy | Should -BeTrue   # the old file is left alone
        }

        It 'does not overwrite an existing Reforge app list' {
            $env:APPDATA = Join-Path $TestDrive 'BothAppData'
            $legacy = Join-Path $env:APPDATA 'GitHubAppUpdater\github-apps.json'
            $current = Join-Path $env:APPDATA 'Reforge\apps.json'
            New-Item -ItemType Directory -Path (Split-Path -Parent $legacy), (Split-Path -Parent $current) -Force | Out-Null
            Write-TestAppList $legacy @([pscustomobject]@{ Name = 'Old'; Repository = 'owner/old' })
            Write-TestAppList $current @([pscustomobject]@{ Name = 'New'; Repository = 'owner/new' })
            Import-Module $ModuleRoot -Force
            (Get-ReforgeApp).Name | Should -Be 'New'
        }
    }
}
