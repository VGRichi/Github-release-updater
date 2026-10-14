Describe 'Installing apps for the first time' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        $originalArch = $env:PROCESSOR_ARCHITECTURE
        $originalToken = $env:GITHUB_TOKEN
        $env:PROCESSOR_ARCHITECTURE = 'AMD64'
        $env:GITHUB_TOKEN = $null
        Import-ReforgeForTest

        $payload = [Text.Encoding]::ASCII.GetBytes('installer bytes')
        $goodDigest = Get-TestDigest $payload
    }
    AfterAll {
        $env:APPDATA = $originalAppData
        $env:PROCESSOR_ARCHITECTURE = $originalArch
        $env:GITHUB_TOKEN = $originalToken
    }

    BeforeEach {
        $config = New-TestConfigPath
        $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-x64-setup.exe' $goodDigest), (New-TestAsset 'PhotoCraft-1.1.0-x64.msi' $goodDigest)

        # Installed apps: an unrelated app before the install, plus PhotoCraft after it
        $other = New-TestProgram 'Other App' '2.0'
        $script:afterInstall = @($other, (New-TestProgram 'PhotoCraft 1.1.0 (x64)' '1.1.0' -Msi))
        $script:installed = @($other)
        Mock -ModuleName Reforge Get-InstalledProgram { $script:installed }
        Mock -ModuleName Reforge Invoke-Installer { $script:installed = $script:afterInstall; 0 }

        Mock -ModuleName Reforge Get-LatestRelease { $release }
        Mock -ModuleName Reforge Test-IsAdmin { $false }
        Mock -ModuleName Reforge Invoke-WebRequest { [IO.File]::WriteAllBytes($OutFile, [Text.Encoding]::ASCII.GetBytes('installer bytes')) }
        Mock -ModuleName Reforge Read-Choice { 0 }   # Yes
    }

    It 'installs the .msi after asking, and tracks the app by its name in Installed apps' {
        $result = Install-ReforgeApp storytold/photocraft -ConfigPath $config
        $result.PSObject.TypeNames | Should -Contain 'Reforge.Result'
        $result.Name | Should -Be 'PhotoCraft (x64)'
        $result.Installed | Should -Be '1.1.0'
        $result.Latest | Should -Be 'v1.1.0'
        $result.Status | Should -Be 'Installed'
        $result.Note | Should -Be 'Checksum verified'
        Should -Invoke Invoke-Installer -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { $File -like '*PhotoCraft-1.1.0-x64.msi' }
        Should -Invoke Read-Choice -ModuleName Reforge -Times 1 -Exactly

        $app = Read-TestAppList $config
        $app.Name | Should -Be 'PhotoCraft (x64)'
        $app.Repository | Should -Be 'storytold/photocraft'
        $app.HadDigest | Should -BeTrue
    }

    It 'can then be updated: Update-ReforgeApp finds the installed app' {
        Install-ReforgeApp storytold/photocraft -ConfigPath $config | Out-Null
        (Update-ReforgeApp -ConfigPath $config).Status | Should -Be 'UpToDate'
    }

    It 'keeps the name given with -Name' {
        (Install-ReforgeApp storytold/photocraft PhotoCraft -ConfigPath $config).Name | Should -Be 'PhotoCraft'
        (Read-TestAppList $config).Name | Should -Be 'PhotoCraft'
    }

    It 'saves -Pattern, -SilentArgs, -Prerelease and -AllowUnverified for later updates' {
        Install-ReforgeApp storytold/photocraft -Pattern '*setup.exe' -SilentArgs '--quiet' -Prerelease -AllowUnverified -ConfigPath $config | Out-Null
        Should -Invoke Invoke-Installer -ModuleName Reforge -ParameterFilter { $File -like '*setup.exe' -and $SilentArgs -eq '--quiet' }
        Should -Invoke Get-LatestRelease -ModuleName Reforge -ParameterFilter { $Prerelease }
        $app = Read-TestAppList $config
        $app.Pattern | Should -Be '*setup.exe'
        $app.SilentArgs | Should -Be '--quiet'
        $app.Prerelease | Should -BeTrue
        $app.AllowUnverified | Should -BeTrue
    }

    It 'installs nothing and tracks nothing when the answer is No' {
        Mock -ModuleName Reforge Read-Choice { 1 }
        $result = Install-ReforgeApp storytold/photocraft -ConfigPath $config
        $result.Status | Should -Be 'Skipped'
        Should -Invoke Invoke-Installer -ModuleName Reforge -Times 0 -Exactly
        $config | Should -Not -Exist
    }

    It 'does not ask with -Force' {
        (Install-ReforgeApp storytold/photocraft -Force -ConfigPath $config).Status | Should -Be 'Installed'
        Should -Invoke Read-Choice -ModuleName Reforge -Times 0 -Exactly
    }

    Context 'Without a checksum' {
        BeforeEach {
            $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-x64.msi')
        }

        It 'asks once, with the unverified-download question, even with -Force' {
            Mock -ModuleName Reforge Read-Choice { 2 }   # No
            (Install-ReforgeApp storytold/photocraft -Force -ConfigPath $config).Status | Should -Be 'Skipped'
            Should -Invoke Read-Choice -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { $Caption -eq 'Unverified download' }
            Should -Invoke Invoke-Installer -ModuleName Reforge -Times 0 -Exactly
        }

        It 'remembers Always as trusting the app' {
            Mock -ModuleName Reforge Read-Choice { 1 }   # Always
            (Install-ReforgeApp storytold/photocraft -ConfigPath $config).Note | Should -Be 'Unverified'
            (Read-TestAppList $config).AllowUnverified | Should -BeTrue
        }

        It 'asks the normal question for a trusted app' {
            Install-ReforgeApp storytold/photocraft -AllowUnverified -ConfigPath $config | Out-Null
            Should -Invoke Read-Choice -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { $Caption -eq 'Install' }
        }
    }

    It 'only reports what it would install with -WhatIf' {
        $result = Install-ReforgeApp storytold/photocraft -WhatIf -ConfigPath $config
        $result.Status | Should -Be 'Available'
        $result.Note | Should -Be 'Checksum available'
        Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 0 -Exactly
        Should -Invoke Read-Choice -ModuleName Reforge -Times 0 -Exactly
        $config | Should -Not -Exist
    }

    It 'reports a failed installer, suggesting admin rights, and tracks nothing' {
        Mock -ModuleName Reforge Invoke-Installer { 1603 }
        $result = Install-ReforgeApp storytold/photocraft -ConfigPath $config
        $result.Status | Should -Be 'Failed'
        $result.Note | Should -BeLike 'Installer exit code 1603*administrator'
        $config | Should -Not -Exist
    }

    It 'warns and tracks it by the repository name when the app does not show up in Installed apps' {
        $script:afterInstall = $script:installed
        $result = Install-ReforgeApp storytold/photocraft -ConfigPath $config -WarningVariable warn 3>$null
        $result.Status | Should -Be 'Installed'
        $result.Name | Should -Be 'photocraft'
        $warn | Should -HaveCount 1
        (Read-TestAppList $config).Name | Should -Be 'photocraft'
    }

    It 'picks the app named like the repository when the installer added several' {
        $script:afterInstall = @($other, (New-TestProgram 'Some Runtime' '9.0'), (New-TestProgram 'PhotoCraft' '1.1.0'))
        (Install-ReforgeApp storytold/photocraft -ConfigPath $config).Name | Should -Be 'PhotoCraft'
    }

    It 'notices an app that was already installed when the installer updates it' {
        $script:installed = @($other, (New-TestProgram 'PhotoCraft' '1.0.0'))
        $script:afterInstall = @($other, (New-TestProgram 'PhotoCraft' '1.1.0'))
        $result = Install-ReforgeApp storytold/photocraft -ConfigPath $config
        $result.Name | Should -Be 'PhotoCraft'
        $result.Installed | Should -Be '1.1.0'
    }

    It 'refuses an app that is already in the list' {
        Write-TestAppList $config @([pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft' })
        Install-ReforgeApp storytold/photocraft -ConfigPath $config -ErrorAction SilentlyContinue -ErrorVariable err | Should -BeNullOrEmpty
        $err | Should -HaveCount 1
        Should -Invoke Get-LatestRelease -ModuleName Reforge -Times 0 -Exactly
    }

    It 'refuses something that is not a GitHub repository' {
        Install-ReforgeApp 'not a repo' -ConfigPath $config -ErrorAction SilentlyContinue -ErrorVariable err | Should -BeNullOrEmpty
        $err | Should -HaveCount 1
    }

    It 'accepts a GitHub link' {
        (Install-ReforgeApp https://github.com/storytold/photocraft -ConfigPath $config).Repository | Should -Be 'storytold/photocraft'
    }

    It 'skips a release without a Windows installer' {
        $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-linux.AppImage')
        $result = Install-ReforgeApp storytold/photocraft -ConfigPath $config
        $result.Status | Should -Be 'Skipped'
        $result.Note | Should -BeLike 'No Windows installer in this release*Add-ReforgeApp -Destination*'
    }

    It 'skips a repository without releases' {
        Mock -ModuleName Reforge Get-LatestRelease { }
        (Install-ReforgeApp storytold/photocraft -ConfigPath $config).Status | Should -Be 'Skipped'
    }

    It 'installs several apps from the pipeline' {
        Mock -ModuleName Reforge Get-InstalledProgram { }
        $results = @(
            [pscustomobject]@{ Repository = 'storytold/photocraft'; Name = 'PhotoCraft' }
            [pscustomobject]@{ Repository = 'storytold/pdfcraft'; Name = 'PdfCraft' }
        ) | Install-ReforgeApp -ConfigPath $config
        $results.Status | Should -Be @('Installed', 'Installed')
        (Read-TestAppList $config).Name | Should -Be @('PhotoCraft', 'PdfCraft')
    }
}
