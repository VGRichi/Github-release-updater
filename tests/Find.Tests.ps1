Describe 'Finding installed GitHub apps' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        Import-ReforgeForTest

        # Find-ReforgeApp only asks winget when Get-WinGetPackage exists. Make sure it does,
        # so the real WinGet module (if installed) is never called and every run is the same.
        $addedWinGetStub = -not (Get-Command Get-WinGetPackage -ErrorAction SilentlyContinue)
        if ($addedWinGetStub) { function global:Get-WinGetPackage { } }
    }
    AfterAll {
        $env:APPDATA = $originalAppData
        if ($addedWinGetStub) { Remove-Item Function:\Get-WinGetPackage }
    }

    Context 'Get-InstalledProgram' {
        It 'returns named programs that are not system components' {
            Mock -ModuleName Reforge Get-ItemProperty {
                [pscustomobject]@{ DisplayName = 'Visible App' }
                [pscustomobject]@{ DisplayName = 'Hidden Runtime'; SystemComponent = 1 }
                [pscustomobject]@{ DisplayName = $null }
            }
            (InModuleScope Reforge { Get-InstalledProgram }).DisplayName | Should -Be 'Visible App'
        }

        It 'reads all three uninstall keys' {
            Mock -ModuleName Reforge Get-ItemProperty { }
            InModuleScope Reforge { Get-InstalledProgram } | Out-Null
            Should -Invoke Get-ItemProperty -ModuleName Reforge -ParameterFilter {
                $Path.Count -eq 3 -and ($Path -like 'HKCU:*').Count -eq 1 -and ($Path -like '*WOW6432Node*').Count -eq 1
            }
        }
    }

    Context 'Find-ReforgeApp' {
        BeforeEach {
            $config = New-TestConfigPath
            Mock -ModuleName Reforge Get-WinGetPackage { }
            Mock -ModuleName Reforge Get-InstalledProgram {
                [pscustomobject]@{ DisplayName = 'PhotoCraft'; DisplayVersion = '1.0.0'; URLInfoAbout = 'https://github.com/storytold/photocraft' }
                [pscustomobject]@{ DisplayName = 'PdfCraft'; DisplayVersion = '2.0'; URLUpdateInfo = 'https://github.com/storytold/pdfcraft.git' }
                [pscustomobject]@{ DisplayName = 'HelpOnly'; DisplayVersion = '3.0'; HelpLink = 'https://github.com/someone/helponly/issues' }
                [pscustomobject]@{ DisplayName = 'PhotoCraft Helper'; DisplayVersion = '1.0.0'; URLInfoAbout = 'https://github.com/storytold/photocraft' }
                [pscustomobject]@{ DisplayName = 'Sponsored'; URLInfoAbout = 'https://github.com/sponsors/someone' }
                [pscustomobject]@{ DisplayName = 'Elsewhere'; URLInfoAbout = 'https://example.com' }
            }
        }

        It 'finds apps from any of the three link fields, once per repository' {
            $found = Find-ReforgeApp -ConfigPath $config
            $found.Repository | Should -Be @('storytold/photocraft', 'storytold/pdfcraft', 'someone/helponly')
            $found[0].Name | Should -Be 'PhotoCraft'
            $found[0].Version | Should -Be '1.0.0'
            $found[0].PSObject.TypeNames | Should -Contain 'Reforge.Candidate'
        }

        It 'leaves out apps already tracked' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft' })
            (Find-ReforgeApp -ConfigPath $config).Repository | Should -Not -Contain 'storytold/photocraft'
        }

        It 'leaves out apps that winget updates' {
            Mock -ModuleName Reforge Get-WinGetPackage { [pscustomobject]@{ Name = 'PdfCraft'; Source = 'winget' } }
            (Find-ReforgeApp -ConfigPath $config).Repository | Should -Not -Contain 'storytold/pdfcraft'
        }

        It 'keeps apps that winget only knows from another source' {
            Mock -ModuleName Reforge Get-WinGetPackage { [pscustomobject]@{ Name = 'PdfCraft'; Source = 'msstore' } }
            (Find-ReforgeApp -ConfigPath $config).Repository | Should -Contain 'storytold/pdfcraft'
        }

        It 'can be piped into Add-ReforgeApp' {
            Find-ReforgeApp -ConfigPath $config | Add-ReforgeApp -ConfigPath $config
            (Get-ReforgeApp -ConfigPath $config).Name | Should -Be @('PhotoCraft', 'PdfCraft', 'HelpOnly')
        }
    }
}
