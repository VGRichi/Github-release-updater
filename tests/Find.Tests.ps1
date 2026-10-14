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

    Context 'Get-ProgramBaseName' {
        It 'turns "<Name>" into "<Expected>"' -TestCases @(
            @{ Name = 'PhotoCraft 1.2.3 (x64)'; Expected = 'PhotoCraft (x64)' }
            @{ Name = '7-Zip 23.01 (x64)'; Expected = '7-Zip (x64)' }
            @{ Name = 'Python 3.12.1 (64-bit)'; Expected = 'Python (64-bit)' }
            @{ Name = 'App v2.0-beta1'; Expected = 'App' }
            @{ Name = 'Notepad++ (64-bit x64)'; Expected = 'Notepad++ (64-bit x64)' }
            @{ Name = 'PhotoCraft'; Expected = 'PhotoCraft' }
        ) {
            InModuleScope Reforge -Parameters @{ N = $Name } { Get-ProgramBaseName $N } | Should -Be $Expected
        }
    }

    Context 'Find-ReforgeApp' {
        BeforeEach {
            $config = New-TestConfigPath
            Mock -ModuleName Reforge Get-GitHubDownload { }   # never scan the real Downloads folder
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
            $found[0].Type | Should -Be 'Installer'
            $found[0].PSObject.TypeNames | Should -Contain 'Reforge.Candidate'
        }

        It 'suggests the name without its version number' {
            Mock -ModuleName Reforge Get-InstalledProgram {
                [pscustomobject]@{ DisplayName = 'PhotoCraft 1.0.0 (x64)'; DisplayVersion = '1.0.0'; URLInfoAbout = 'https://github.com/storytold/photocraft' }
            }
            (Find-ReforgeApp -ConfigPath $config).Name | Should -Be 'PhotoCraft (x64)'
        }

        It 'leaves out apps already tracked' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft' })
            (Find-ReforgeApp -ConfigPath $config).Repository | Should -Not -Contain 'storytold/photocraft'
        }

        It 'leaves out apps that winget updates' {
            Mock -ModuleName Reforge Get-WinGetPackage { [pscustomobject]@{ Name = 'PdfCraft'; InstalledVersion = '2.0'; Source = 'winget' } }
            (Find-ReforgeApp -ConfigPath $config).Repository | Should -Not -Contain 'storytold/pdfcraft'
        }

        It 'leaves out apps that winget knows by a shorter name' {
            # Like PowerShell: "PowerShell 7-x64" in Installed apps, "PowerShell" in winget
            Mock -ModuleName Reforge Get-InstalledProgram {
                [pscustomobject]@{ DisplayName = 'PowerShell 7-x64'; DisplayVersion = '7.6.6.0'; URLInfoAbout = 'https://github.com/PowerShell/PowerShell' }
            }
            Mock -ModuleName Reforge Get-WinGetPackage { [pscustomobject]@{ Name = 'PowerShell'; InstalledVersion = '7.6.6.0'; Source = 'winget' } }
            Find-ReforgeApp -ConfigPath $config | Should -BeNullOrEmpty
        }

        It 'keeps an app whose name only starts like a winget app''s, with another version' {
            # winget's "Photo" isn't PhotoCraft
            Mock -ModuleName Reforge Get-WinGetPackage { [pscustomobject]@{ Name = 'Photo'; InstalledVersion = '9.9'; Source = 'winget' } }
            (Find-ReforgeApp -ConfigPath $config).Repository | Should -Contain 'storytold/photocraft'
        }

        It 'keeps apps that winget only knows from another source' {
            Mock -ModuleName Reforge Get-WinGetPackage { [pscustomobject]@{ Name = 'PdfCraft'; InstalledVersion = '2.0'; Source = 'msstore' } }
            (Find-ReforgeApp -ConfigPath $config).Repository | Should -Contain 'storytold/pdfcraft'
        }

        It 'can be piped into Add-ReforgeApp' {
            Find-ReforgeApp -ConfigPath $config | Add-ReforgeApp -ConfigPath $config
            (Get-ReforgeApp -ConfigPath $config).Name | Should -Be @('PhotoCraft', 'PdfCraft', 'HelpOnly')
        }
    }

    Context 'Downloads from GitHub' {
        BeforeAll {
            # A downloaded file with the note browsers add: GitHub's release page as ReferrerUrl,
            # and the file host (which doesn't name the repository) as HostUrl
            function New-Download([string]$Folder, [string]$Name, [string]$Referrer, [int]$DaysOld = 0) {
                $file = Join-Path $Folder $Name
                New-Item -ItemType Directory -Path (Split-Path -Parent $file) -Force | Out-Null
                Set-Content -LiteralPath $file -Value 'bytes'
                $note = "[ZoneTransfer]`r`nZoneId=3`r`nReferrerUrl=$Referrer`r`nHostUrl=https://release-assets.githubusercontent.com/github-production-release-asset/1/2?sig=x"
                Set-Content -LiteralPath $file -Stream Zone.Identifier -Value $note
                (Get-Item -LiteralPath $file).LastWriteTime = (Get-Date).AddDays(-$DaysOld)
                $file
            }
        }

        BeforeEach {
            $config = New-TestConfigPath
            $downloads = Join-Path $TestDrive ('dl-' + [guid]::NewGuid().ToString('N').Substring(0, 6))
            New-Item -ItemType Directory -Path $downloads | Out-Null
            Mock -ModuleName Reforge Get-WinGetPackage { }
            Mock -ModuleName Reforge Get-InstalledProgram {
                [pscustomobject]@{ DisplayName = 'GridCraft'; DisplayVersion = '0.5.0'; URLInfoAbout = 'https://getartcraft.com' }
                [pscustomobject]@{ DisplayName = 'Notepad++ (64-bit x64)'; DisplayVersion = '8.9.8.1' }
            }
        }

        Context 'Get-DownloadOrigin' {
            It 'reads the repository and tag from the release page the file came from' {
                $file = New-Download $downloads 'tool.7z' 'https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.4'
                $origin = InModuleScope Reforge -Parameters @{ F = $file } { Get-DownloadOrigin $F }
                $origin.Repository | Should -Be 'optiscaler/OptiScaler'
                $origin.Tag | Should -Be 'v0.9.4'
            }

            It 'reads a direct GitHub download link too' {
                $file = Join-Path $downloads 'direct.zip'
                Set-Content -LiteralPath $file -Value 'bytes'
                Set-Content -LiteralPath $file -Stream Zone.Identifier -Value "[ZoneTransfer]`r`nZoneId=3`r`nHostUrl=https://github.com/doitsujin/dxvk/releases/download/v2.5/dxvk-2.5.tar.gz"
                (InModuleScope Reforge -Parameters @{ F = $file } { Get-DownloadOrigin $F }).Tag | Should -Be 'v2.5'
            }

            It 'returns nothing for <Case>' -TestCases @(
                @{ Case = 'a file from another site'; Referrer = 'https://example.com/tool.zip' }
                @{ Case = 'source code from a repository page'; Referrer = 'https://github.com/owner/repo/archive/refs/tags/v1.zip' }
            ) {
                $file = New-Download $downloads 'other.zip' $Referrer
                InModuleScope Reforge -Parameters @{ F = $file } { Get-DownloadOrigin $F } | Should -BeNullOrEmpty
            }

            It 'returns nothing for a file without a note' {
                $file = Join-Path $downloads 'plain.zip'
                Set-Content -LiteralPath $file -Value 'bytes'
                InModuleScope Reforge -Parameters @{ F = $file } { Get-DownloadOrigin $F } | Should -BeNullOrEmpty
            }
        }

        It 'suggests a downloaded archive tool, with the version of the newest download' {
            New-Download $downloads 'Optiscaler_0.9.3.7z' 'https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.3' -DaysOld 30 | Out-Null
            New-Download $downloads 'Mods\Optiscaler_0.9.4.7z' 'https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.4' | Out-Null
            $found = @(Find-ReforgeApp $downloads -ConfigPath $config)
            $found | Should -HaveCount 1
            $found[0].Name | Should -Be 'OptiScaler'
            $found[0].Repository | Should -Be 'optiscaler/OptiScaler'
            $found[0].Version | Should -Be 'v0.9.4'
            $found[0].Type | Should -Be 'Archive'
        }

        It 'suggests an installed app whose installer was downloaded, by its installed name' {
            New-Download $downloads 'GridCraft_0.5.0_x64-setup.exe' 'https://github.com/storytold/gridcraft/releases/tag/v0.5.0' | Out-Null
            $found = Find-ReforgeApp $downloads -ConfigPath $config
            $found.Name | Should -Be 'GridCraft'
            $found.Repository | Should -Be 'storytold/gridcraft'
            $found.Version | Should -Be '0.5.0'
            $found.Type | Should -Be 'Installer'
        }

        It 'matches an app by its first word and a version written differently' {
            New-Download $downloads 'npp.8.9.8.1.Installer.x64.exe' 'https://github.com/notepad-plus-plus/notepad-plus-plus/releases/tag/v8.9.8.1' | Out-Null
            (Find-ReforgeApp $downloads -ConfigPath $config).Name | Should -Be 'Notepad++ (64-bit x64)'
        }

        It 'ignores a downloaded installer when <Case>' -TestCases @(
            @{ Case = 'its app is not installed'; File = 'PhotoCraft-1.0.0-setup.exe'; Referrer = 'https://github.com/storytold/photocraft/releases/tag/v1.0.0' }
            @{ Case = 'the installed version is different'; File = 'GridCraft_0.4.0_x64-setup.exe'; Referrer = 'https://github.com/storytold/gridcraft/releases/tag/v0.4.0' }
        ) {
            New-Download $downloads $File $Referrer | Out-Null
            Find-ReforgeApp $downloads -ConfigPath $config | Should -BeNullOrEmpty
        }

        It 'ignores downloads of apps already tracked, and other files' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'OptiScaler (Game)'; Repository = 'optiscaler/OptiScaler'; Type = 'archive' })
            New-Download $downloads 'Optiscaler_0.9.4.7z' 'https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.4' | Out-Null
            New-Download $downloads 'notes.txt' 'https://github.com/owner/repo/releases/tag/v1.0' | Out-Null
            Find-ReforgeApp $downloads -ConfigPath $config | Should -BeNullOrEmpty
        }

        It 'only adds a found archive tool with -Destination' {
            New-Download $downloads 'Optiscaler_0.9.4.7z' 'https://github.com/optiscaler/OptiScaler/releases/tag/v0.9.4' | Out-Null
            Find-ReforgeApp $downloads -ConfigPath $config | Add-ReforgeApp -ConfigPath $config -ErrorAction SilentlyContinue -ErrorVariable err
            $err | Should -HaveCount 1
            $config | Should -Not -Exist

            $game = New-Item -ItemType Directory -Path (Join-Path $TestDrive ([guid]::NewGuid().ToString('N').Substring(0, 6)))
            Find-ReforgeApp $downloads -ConfigPath $config | Add-ReforgeApp -Destination $game.FullName -ConfigPath $config
            (Get-ReforgeApp -ConfigPath $config).Type | Should -Be 'Archive'
        }
    }
}
