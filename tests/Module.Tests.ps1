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

        It 'describes <Command> with its own synopsis on the module help page' -TestCases @(
            @{ Command = 'Add-ReforgeApp' }, @{ Command = 'Get-ReforgeApp' }, @{ Command = 'Remove-ReforgeApp' }
            @{ Command = 'Find-ReforgeApp' }, @{ Command = 'Update-ReforgeApp' }
        ) {
            $page = Get-Content -LiteralPath (Join-Path $repoRoot 'docs\en-US\Reforge.md') -Raw
            $synopsis = (Get-Help $Command).Synopsis.Trim()
            $page | Should -Match ([regex]::Escape("### [$Command]($Command.md)") + '\s+' + [regex]::Escape($synopsis))
        }
    }

    Context 'Importing' {
        It 'creates nothing in APPDATA' {
            $env:APPDATA = Join-Path $TestDrive 'EmptyAppData'
            New-Item -ItemType Directory -Path $env:APPDATA | Out-Null
            Import-Module $ModuleRoot -Force
            Get-ChildItem -LiteralPath $env:APPDATA | Should -BeNullOrEmpty
        }
    }
}
