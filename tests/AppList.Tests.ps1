Describe 'The app list' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        Import-ReforgeForTest
    }
    AfterAll { $env:APPDATA = $originalAppData }

    BeforeEach { $config = New-TestConfigPath }

    Context 'Read-AppList and Save-AppList' {
        It 'returns nothing for a missing, empty or blank file' {
            InModuleScope Reforge -Parameters @{ P = $config } { Read-AppList $P } | Should -BeNullOrEmpty
            Set-Content -LiteralPath $config -Value ''
            InModuleScope Reforge -Parameters @{ P = $config } { Read-AppList $P } | Should -BeNullOrEmpty
            Set-Content -LiteralPath $config -Value "  `r`n "
            InModuleScope Reforge -Parameters @{ P = $config } { Read-AppList $P } | Should -BeNullOrEmpty
        }

        It 'renames the v1 "Repo" field to "Repository"' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'Old'; Repo = 'owner/old' })
            $app = InModuleScope Reforge -Parameters @{ P = $config } { Read-AppList $P }
            $app.Repository | Should -Be 'owner/old'
            $app.PSObject.Properties['Repo'] | Should -BeNullOrEmpty
        }

        It 'merges the v1 "Destinations" field into "Destination"' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'Tool'; Repository = 'o/tool'; Destination = 'C:\A'; Destinations = @('C:\B', 'C:\C') })
            $app = InModuleScope Reforge -Parameters @{ P = $config } { Read-AppList $P }
            @($app.Destination) | Should -Be @('C:\A', 'C:\B', 'C:\C')
            $app.PSObject.Properties['Destinations'] | Should -BeNullOrEmpty
        }

        It 'replaces an existing list without leaving a temp file behind' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'Old'; Repository = 'o/old' })
            InModuleScope Reforge -Parameters @{ P = $config } { Save-AppList $P @([pscustomobject]@{ Name = 'New'; Repository = 'o/new' }) }
            (Read-TestAppList $config).Name | Should -Be 'New'
            "$config.tmp" | Should -Not -Exist
        }

        It 'accepts a path relative to the current folder' {
            Push-Location $TestDrive
            try { InModuleScope Reforge { Save-AppList '.\relative-apps.json' @([pscustomobject]@{ Name = 'One'; Repository = 'o/one' }) } }
            finally { Pop-Location }
            Join-Path $TestDrive 'relative-apps.json' | Should -Exist
        }

        It 'writes a JSON array even for one app, creating the folder' {
            $nested = Join-Path $TestDrive ('new-' + [guid]::NewGuid().ToString('N') + '\apps.json')
            InModuleScope Reforge -Parameters @{ P = $nested } { Save-AppList $P @([pscustomobject]@{ Name = 'One'; Repository = 'o/one' }) }
            (Get-Content -LiteralPath $nested -Raw).TrimStart().StartsWith('[') | Should -BeTrue
        }
    }

    Context 'Add-ReforgeApp' {
        It 'tracks owner/repo, named after the repository by default' {
            Add-ReforgeApp storytold/photocraft -ConfigPath $config
            $app = Get-ReforgeApp -ConfigPath $config
            $app.Name | Should -Be 'photocraft'
            $app.Repository | Should -Be 'storytold/photocraft'
            $app.Type | Should -Be 'Installer'
        }

        It 'stores only the fields that were given' {
            Add-ReforgeApp storytold/photocraft PhotoCraft -ConfigPath $config
            (Read-TestAppList $config).PSObject.Properties.Name | Should -Be @('Name', 'Repository')
        }

        It 'accepts the GitHub link "<Link>"' -TestCases @(
            @{ Link = 'https://github.com/storytold/photocraft' }
            @{ Link = 'https://www.github.com/storytold/photocraft.git' }
            @{ Link = 'http://github.com/storytold/photocraft/' }
            @{ Link = '  storytold/photocraft  ' }
        ) {
            Add-ReforgeApp $Link -ConfigPath $config
            (Get-ReforgeApp -ConfigPath $config).Repository | Should -Be 'storytold/photocraft'
        }

        It 'rejects "<Link>" and saves nothing' -TestCases @(
            @{ Link = 'photocraft' }
            @{ Link = 'storytold/photocraft/releases' }
            @{ Link = 'https://gitlab.com/storytold/photocraft' }
        ) {
            Add-ReforgeApp $Link -ConfigPath $config -ErrorAction SilentlyContinue -ErrorVariable err
            $err | Should -HaveCount 1
            Test-Path -LiteralPath $config | Should -BeFalse
        }

        It 'refuses a name that is already in the list' {
            Add-ReforgeApp storytold/photocraft PhotoCraft -ConfigPath $config
            Add-ReforgeApp someone/else photocraft -ConfigPath $config -ErrorAction SilentlyContinue -ErrorVariable err
            $err | Should -HaveCount 1
            (Get-ReforgeApp -ConfigPath $config).Repository | Should -Be 'storytold/photocraft'
        }

        It 'replaces an app with -Force, keeping what it learned when the repository is the same' {
            $dest = New-Item -ItemType Directory -Path (Join-Path $TestDrive ([guid]::NewGuid().ToString('N')))
            Write-TestAppList $config @([pscustomobject]@{
                Name = 'DXVK'; Repository = 'doitsujin/dxvk'; Type = 'archive'; Destination = @($dest.FullName)
                HadDigest = $true; InstalledVersion = 'v2.4'; AllowUnverified = $true
            })
            Add-ReforgeApp doitsujin/dxvk DXVK -Destination $dest.FullName -Subfolder x64 -Force -ConfigPath $config
            $app = Read-TestAppList $config
            $app.HadDigest | Should -BeTrue
            $app.InstalledVersion | Should -Be 'v2.4'
            $app.Subfolder | Should -Be 'x64'
            $app.PSObject.Properties['AllowUnverified'] | Should -BeNullOrEmpty   # trust must be given again
        }

        It 'forgets what it learned when -Force points the name at another repository' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'App'; Repository = 'old/app'; HadDigest = $true })
            Add-ReforgeApp new/app App -Force -ConfigPath $config
            (Read-TestAppList $config).PSObject.Properties['HadDigest'] | Should -BeNullOrEmpty
        }

        It 'makes an archive tool when -Destination is given, storing the full path' {
            $dest = New-Item -ItemType Directory -Path (Join-Path $TestDrive ([guid]::NewGuid().ToString('N')))
            Push-Location $dest.Parent.FullName
            try { Add-ReforgeApp doitsujin/dxvk -Destination (".\" + $dest.Name) -ConfigPath $config }
            finally { Pop-Location }
            $app = Read-TestAppList $config
            $app.Type | Should -Be 'archive'
            @($app.Destination) | Should -Be @($dest.FullName)
        }

        It 'refuses a -Destination folder that does not exist' {
            Add-ReforgeApp doitsujin/dxvk -Destination (Join-Path $TestDrive 'nope') -ConfigPath $config -ErrorAction SilentlyContinue -ErrorVariable err
            $err | Should -HaveCount 1
            Test-Path -LiteralPath $config | Should -BeFalse
        }

        It 'takes pipeline input by property name, including the old "Repo" name' {
            @(
                [pscustomobject]@{ Repository = 'owner/one'; Name = 'One' }
                [pscustomobject]@{ Repo = 'owner/two' }
            ) | Add-ReforgeApp -ConfigPath $config
            (Get-ReforgeApp -ConfigPath $config).Name | Should -Be @('One', 'two')
        }

        It 'outputs the app with -PassThru, and nothing without it' {
            Add-ReforgeApp owner/one -ConfigPath $config | Should -BeNullOrEmpty
            $out = Add-ReforgeApp owner/two -PassThru -ConfigPath $config
            $out.PSObject.TypeNames | Should -Contain 'Reforge.App'
            $out.Name | Should -Be 'two'
        }

        It 'stores -Rename so it can be read back' {
            $dest = New-Item -ItemType Directory -Path (Join-Path $TestDrive ([guid]::NewGuid().ToString('N')))
            Add-ReforgeApp optiscaler/OptiScaler -Destination $dest.FullName -Rename @{ 'OptiScaler.dll' = 'dxgi.dll' } -Keep '*.ini' -ConfigPath $config
            $app = Read-TestAppList $config
            $app.Rename.'OptiScaler.dll' | Should -Be 'dxgi.dll'
            @($app.Keep) | Should -Be @('*.ini')
        }
    }

    Context 'Get-ReforgeApp' {
        BeforeEach {
            Write-TestAppList $config @(
                [pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft'; AllowUnverified = $true }
                [pscustomobject]@{ Name = 'DXVK (Witcher)'; Repository = 'doitsujin/dxvk'; Type = 'archive'; Destination = @('C:\A', 'C:\B'); InstalledVersion = 'v2.5' }
                [pscustomobject]@{ Name = 'DXVK (Cyberpunk)'; Repository = 'doitsujin/dxvk'; Type = 'archive'; Destinations = @('C:\C') }
            )
        }

        It 'lists every app' {
            Get-ReforgeApp -ConfigPath $config | Should -HaveCount 3
        }

        It 'filters by name with wildcards' {
            (Get-ReforgeApp 'DXVK*' -ConfigPath $config).Name | Should -Be @('DXVK (Witcher)', 'DXVK (Cyberpunk)')
        }

        It 'shows type, destinations, version and trust' {
            $apps = Get-ReforgeApp -ConfigPath $config
            $apps[0].Type | Should -Be 'Installer'
            $apps[0].Trusted | Should -BeTrue
            $apps[1].Type | Should -Be 'Archive'
            $apps[1].Destination | Should -Be 'C:\A; C:\B'
            $apps[1].InstalledVersion | Should -Be 'v2.5'
            $apps[1].Trusted | Should -BeFalse
        }

        It 'reads the old "Destinations" field too' {
            (Get-ReforgeApp 'DXVK (Cyberpunk)' -ConfigPath $config).Destination | Should -Be 'C:\C'
        }

        It 'returns nothing when there is no list yet' {
            Get-ReforgeApp -ConfigPath (New-TestConfigPath) | Should -BeNullOrEmpty
        }
    }

    Context 'Remove-ReforgeApp' {
        BeforeEach {
            Write-TestAppList $config @(
                [pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft' }
                [pscustomobject]@{ Name = 'DXVK (Witcher)'; Repository = 'doitsujin/dxvk' }
                [pscustomobject]@{ Name = 'DXVK (Cyberpunk)'; Repository = 'doitsujin/dxvk' }
            )
        }

        It 'stops tracking an app' {
            Remove-ReforgeApp PhotoCraft -ConfigPath $config
            (Get-ReforgeApp -ConfigPath $config).Name | Should -Not -Contain 'PhotoCraft'
        }

        It 'removes every app matching a wildcard' {
            Remove-ReforgeApp 'DXVK*' -ConfigPath $config
            (Get-ReforgeApp -ConfigPath $config).Name | Should -Be 'PhotoCraft'
        }

        It 'takes apps from Get-ReforgeApp through the pipeline' {
            Get-ReforgeApp 'DXVK (W*' -ConfigPath $config | Remove-ReforgeApp -ConfigPath $config
            Get-ReforgeApp -ConfigPath $config | Should -HaveCount 2
        }

        It 'leaves an empty list after removing the last app' {
            Remove-ReforgeApp '*' -ConfigPath $config
            # An empty JSON array (Windows PowerShell writes it over several lines)
            (Get-Content -LiteralPath $config -Raw) -replace '\s' | Should -Be '[]'
            Get-ReforgeApp -ConfigPath $config | Should -BeNullOrEmpty
        }

        It 'reports an unknown name and changes nothing' {
            $before = Get-Content -LiteralPath $config -Raw
            Remove-ReforgeApp 'Nope' -ConfigPath $config -ErrorAction SilentlyContinue -ErrorVariable err
            $err | Should -HaveCount 1
            Get-Content -LiteralPath $config -Raw | Should -Be $before
        }

        It 'changes nothing with -WhatIf' {
            $before = Get-Content -LiteralPath $config -Raw
            Remove-ReforgeApp '*' -WhatIf -ConfigPath $config
            Get-Content -LiteralPath $config -Raw | Should -Be $before
        }
    }
}
