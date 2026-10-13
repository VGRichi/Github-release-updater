Describe 'Updating installer apps' {
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

    Context 'Invoke-Installer' {
        BeforeAll {
            function New-FakeProcess([int]$ExitCode) {
                $p = [pscustomobject]@{ ExitCode = $ExitCode; Handle = [IntPtr]1 }
                $p | Add-Member -MemberType ScriptMethod -Name WaitForExit -Value { }
                $p
            }
            Mock -ModuleName Reforge Start-Process { New-FakeProcess 0 }
            function New-InstallerFile([string]$Name, [string]$Marker) {
                $path = Join-Path $TestDrive $Name
                [IO.File]::WriteAllBytes($path, [Text.Encoding]::ASCII.GetBytes("MZ...$Marker..."))
                $path
            }
        }

        It 'installs an .msi quietly with msiexec' {
            $file = New-InstallerFile 'App.msi' ''
            InModuleScope Reforge -Parameters @{ F = $file } { Invoke-Installer $F '' } | Should -Be 0
            Should -Invoke Start-Process -ModuleName Reforge -Times 1 -Exactly -ParameterFilter {
                $FilePath -eq 'msiexec.exe' -and $ArgumentList -eq "/i `"$file`" /qn /norestart"
            }
        }

        It 'detects an Inno Setup installer' {
            $file = New-InstallerFile 'inno.exe' 'Inno Setup'
            InModuleScope Reforge -Parameters @{ F = $file } { Invoke-Installer $F '' } | Out-Null
            Should -Invoke Start-Process -ModuleName Reforge -ParameterFilter { $ArgumentList -eq '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART' }
        }

        It 'detects an NSIS installer' {
            $file = New-InstallerFile 'nsis.exe' 'Nullsoft Install System'
            InModuleScope Reforge -Parameters @{ F = $file } { Invoke-Installer $F '' } | Out-Null
            Should -Invoke Start-Process -ModuleName Reforge -ParameterFilter { $ArgumentList -eq '/S' }
        }

        It 'uses -SilentArgs instead of detecting' {
            $file = New-InstallerFile 'custom.exe' 'Inno Setup'
            InModuleScope Reforge -Parameters @{ F = $file } { Invoke-Installer $F '--silent' } | Out-Null
            Should -Invoke Start-Process -ModuleName Reforge -ParameterFilter { $ArgumentList -eq '--silent' }
        }

        It 'opens an unknown installer normally, with a warning' {
            $file = New-InstallerFile 'mystery.exe' 'something'
            InModuleScope Reforge -Parameters @{ F = $file } { Invoke-Installer $F '' 3>&1 } |
                Where-Object { $_ -is [Management.Automation.WarningRecord] } | Should -HaveCount 1
            Should -Invoke Start-Process -ModuleName Reforge -ParameterFilter { $FilePath -eq $file -and -not $ArgumentList }
        }

        It 'returns the installer exit code' {
            Mock -ModuleName Reforge Start-Process { New-FakeProcess 1603 }
            $file = New-InstallerFile 'fail.msi' ''
            InModuleScope Reforge -Parameters @{ F = $file } { Invoke-Installer $F '' } | Should -Be 1603
        }

        It 'never waits for the processes an installer starts' {
            Should -Invoke Start-Process -ModuleName Reforge -Times 0 -ParameterFilter { $Wait }
        }
    }

    Context 'Start-InstallerProcess' {
        It 'returns when the installer exits, even if it started a program that keeps running' {
            # The "installer" starts a long-running program (like an app opened after setup) and exits with 7
            $pidFile = Join-Path $TestDrive 'child.pid'
            $installer = "`$c = Start-Process powershell.exe -ArgumentList '-NoProfile', '-Command', 'Start-Sleep 60' -WindowStyle Hidden -PassThru; Set-Content -LiteralPath '$pidFile' `$c.Id; exit 7"
            $watch = [Diagnostics.Stopwatch]::StartNew()
            try {
                InModuleScope Reforge -Parameters @{ S = $installer } { Start-InstallerProcess 'powershell.exe' "-NoProfile -WindowStyle Hidden -Command $S" } | Should -Be 7
                $watch.Elapsed.TotalSeconds | Should -BeLessThan 45
            } finally {
                if (Test-Path -LiteralPath $pidFile) { Stop-Process -Id (Get-Content -LiteralPath $pidFile) -Force -ErrorAction SilentlyContinue }
            }
        }
    }

    Context 'Update-ReforgeApp' {
        BeforeEach {
            $config = New-TestConfigPath
            Write-TestAppList $config @([pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft' })

            $installed = New-TestProgram 'PhotoCraft' '1.0.0'
            $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-x64-setup.exe' $goodDigest), (New-TestAsset 'PhotoCraft-1.1.0-linux.AppImage')

            Mock -ModuleName Reforge Get-InstalledProgram { $installed }
            Mock -ModuleName Reforge Get-LatestRelease { $release }
            Mock -ModuleName Reforge Test-IsAdmin { $false }
            Mock -ModuleName Reforge Invoke-WebRequest { [IO.File]::WriteAllBytes($OutFile, [Text.Encoding]::ASCII.GetBytes('installer bytes')) }
            Mock -ModuleName Reforge Invoke-Installer { 0 }
            Mock -ModuleName Reforge Read-Choice { throw 'Unexpected prompt' }
        }

        It 'installs a verified update' {
            $result = Update-ReforgeApp -ConfigPath $config
            $result.PSObject.TypeNames | Should -Contain 'Reforge.Result'
            $result.Name | Should -Be 'PhotoCraft'
            $result.Repository | Should -Be 'storytold/photocraft'
            $result.Installed | Should -Be '1.0.0'
            $result.Latest | Should -Be 'v1.1.0'
            $result.Status | Should -Be 'Updated'
            $result.Note | Should -Be 'Checksum verified'
            Should -Invoke Invoke-Installer -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { $File -like '*PhotoCraft-1.1.0-x64-setup.exe' }
        }

        It 'deletes the downloaded installer afterwards' {
            Update-ReforgeApp -ConfigPath $config | Out-Null
            Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { -not (Test-Path -LiteralPath (Split-Path -Parent $OutFile)) }
        }

        It 'remembers that this app publishes checksums' {
            Update-ReforgeApp -ConfigPath $config | Out-Null
            (Read-TestAppList $config).HadDigest | Should -BeTrue
        }

        It 'reports UpToDate when the installed version is the latest or newer' {
            $installed = New-TestProgram 'PhotoCraft' '1.1.0'
            (Update-ReforgeApp -ConfigPath $config).Status | Should -Be 'UpToDate'
            Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 0 -Exactly
        }

        It 'reports NotInstalled when no installed app has the name' {
            $installed = New-TestProgram 'Something Else' '1.0.0'
            $result = Update-ReforgeApp -ConfigPath $config
            $result.Status | Should -Be 'NotInstalled'
            Should -Invoke Get-LatestRelease -ModuleName Reforge -Times 0 -Exactly
        }

        It 'matches a partial installed name' {
            $installed = New-TestProgram 'PhotoCraft (64-bit)' '1.0.0'
            (Update-ReforgeApp -ConfigPath $config).Status | Should -Be 'Updated'
        }

        It 'still finds the app after an update changes a version number in its name' {
            # Find-ReforgeApp | Add-ReforgeApp stores the full DisplayName, which often contains the version
            Write-TestAppList $config @([pscustomobject]@{ Name = 'PhotoCraft 1.0.0 (x64)'; Repository = 'storytold/photocraft' })
            $installed = New-TestProgram 'PhotoCraft 1.1.0 (x64)' '1.1.0'
            (Update-ReforgeApp -ConfigPath $config).Status | Should -Not -Be 'NotInstalled'
        }

        It "skips when the versions can't be compared" {
            $release = New-TestRelease 'nightly' -Assets (New-TestAsset 'PhotoCraft-x64-setup.exe' $goodDigest)
            $result = Update-ReforgeApp -ConfigPath $config
            $result.Status | Should -Be 'Skipped'
            $result.Note | Should -Be "Can't compare versions"
        }

        It 'skips when the release has no Windows installer' {
            $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-linux.AppImage')
            $result = Update-ReforgeApp -ConfigPath $config
            $result.Status | Should -Be 'Skipped'
            $result.Note | Should -Be 'No matching Windows installer in this release'
        }

        It 'skips when the repository has no releases' {
            $release = $null
            $result = Update-ReforgeApp -ConfigPath $config
            $result.Status | Should -Be 'Skipped'
            $result.Note | Should -Be 'No releases found'
        }

        It 'only reports the update with -WhatIf' {
            $result = Update-ReforgeApp -ConfigPath $config -WhatIf
            $result.Status | Should -Be 'UpdateAvailable'
            $result.Note | Should -Be 'Checksum available'
            Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 0 -Exactly
            Should -Invoke Invoke-Installer -ModuleName Reforge -Times 0 -Exactly
        }

        It 'does not write the app list with -WhatIf' {
            $before = Get-Content -LiteralPath $config -Raw
            Update-ReforgeApp -ConfigPath $config -WhatIf | Out-Null
            Get-Content -LiteralPath $config -Raw | Should -Be $before
        }

        It 'needs admin rights for an app installed for all users' {
            $installed = New-TestProgram 'PhotoCraft' '1.0.0' -AllUsers
            $result = Update-ReforgeApp -ConfigPath $config
            $result.Status | Should -Be 'NeedsAdmin'
            Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 0 -Exactly
        }

        It 'updates an app installed for all users when running as admin' {
            $installed = New-TestProgram 'PhotoCraft' '1.0.0' -AllUsers
            Mock -ModuleName Reforge Test-IsAdmin { $true }
            (Update-ReforgeApp -ConfigPath $config).Status | Should -Be 'Updated'
        }

        It 'uses the .msi when the app was installed with one' {
            $installed = New-TestProgram 'PhotoCraft' '1.0.0' -Msi
            $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-x64-setup.exe' $goodDigest), (New-TestAsset 'PhotoCraft-1.1.0-x64.msi' $goodDigest)
            Update-ReforgeApp -ConfigPath $config | Out-Null
            Should -Invoke Invoke-Installer -ModuleName Reforge -ParameterFilter { $File -like '*.msi' }
        }

        It 'passes the app''s -SilentArgs to the installer' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft'; SilentArgs = '--silent' })
            Update-ReforgeApp -ConfigPath $config | Out-Null
            Should -Invoke Invoke-Installer -ModuleName Reforge -ParameterFilter { $SilentArgs -eq '--silent' }
        }

        It 'reports <Code> as "<Status>: <Note>"' -TestCases @(
            @{ Code = 3010; Status = 'Updated'; Note = 'Restart needed to finish' }
            @{ Code = 1641; Status = 'Updated'; Note = 'Restart needed to finish' }
            @{ Code = 1603; Status = 'Failed'; Note = 'Installer exit code 1603' }
        ) {
            Mock -ModuleName Reforge Invoke-Installer { $Code }
            $result = Update-ReforgeApp -ConfigPath $config
            $result.Status | Should -Be $Status
            $result.Note | Should -Be $Note
        }

        It 'fails without installing when the checksum does not match' {
            $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-x64-setup.exe' (Get-TestDigest ([byte[]](1, 2, 3))))
            $result = Update-ReforgeApp -ConfigPath $config 3>$null
            $result.Status | Should -Be 'Failed'
            $result.Note | Should -BeLike '*Checksum mismatch*'
            Should -Invoke Invoke-Installer -ModuleName Reforge -Times 0 -Exactly
        }

        It 'skips an unverified download when the answer is No' {
            $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-x64-setup.exe')
            Mock -ModuleName Reforge Read-Choice { 2 }
            $result = Update-ReforgeApp -ConfigPath $config
            $result.Status | Should -Be 'Skipped'
            $result.Note | Should -Be 'Unverified download declined'
            Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 0 -Exactly
        }

        It 'installs an unverified download on Yes and says so' {
            $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-x64-setup.exe')
            Mock -ModuleName Reforge Read-Choice { 0 }
            $result = Update-ReforgeApp -ConfigPath $config
            $result.Status | Should -Be 'Updated'
            $result.Note | Should -Be 'Unverified'
            (Read-TestAppList $config).PSObject.Properties['AllowUnverified'] | Should -BeNullOrEmpty
        }

        It 'saves the trust when the answer is Always' {
            $release = New-TestRelease 'v1.1.0' -Assets (New-TestAsset 'PhotoCraft-1.1.0-x64-setup.exe')
            Mock -ModuleName Reforge Read-Choice { 1 }
            Update-ReforgeApp -ConfigPath $config | Out-Null
            (Read-TestAppList $config).AllowUnverified | Should -BeTrue
        }

        It 'reports a GitHub error as Failed and carries on with the next app' {
            Write-TestAppList $config @(
                [pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft' }
                [pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft-mirror' }
            )
            Mock -ModuleName Reforge Get-LatestRelease -ParameterFilter { $Repository -eq 'storytold/photocraft' } { throw 'API rate limit exceeded' }
            $results = Update-ReforgeApp -ConfigPath $config -WarningVariable warnings 3>$null
            $results[0].Status | Should -Be 'Failed'
            $results[0].Note | Should -Be 'API rate limit exceeded'
            $results[1].Status | Should -Be 'Updated'
            $warnings | Should -HaveCount 1
        }

        It 'passes -Prerelease and the token on to GitHub' {
            Write-TestAppList $config @([pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft'; Prerelease = $true })
            $token = ConvertTo-SecureString 'test-token' -AsPlainText -Force
            Update-ReforgeApp -ConfigPath $config -Token $token | Out-Null
            Should -Invoke Get-LatestRelease -ModuleName Reforge -ParameterFilter {
                $Repository -eq 'storytold/photocraft' -and $Prerelease -and $Headers.Authorization -eq 'Bearer test-token'
            }
        }

        It 'only updates the apps named, with wildcards' {
            Write-TestAppList $config @(
                [pscustomobject]@{ Name = 'PhotoCraft'; Repository = 'storytold/photocraft' }
                [pscustomobject]@{ Name = 'PdfCraft'; Repository = 'storytold/pdfcraft' }
            )
            (Update-ReforgeApp 'Photo*' -ConfigPath $config).Name | Should -Be 'PhotoCraft'
        }

        It 'takes names from Get-ReforgeApp through the pipeline' {
            (Get-ReforgeApp -ConfigPath $config | Update-ReforgeApp -ConfigPath $config).Status | Should -Be 'Updated'
        }

        Context 'leftover work folders' {
            BeforeEach {
                $workRoot = Join-Path $TestDrive ([guid]::NewGuid().ToString('N'))
                $stale = New-Item -ItemType Directory -Path (Join-Path $workRoot 'reforge-0123abcd')
                $stale.LastWriteTime = (Get-Date).AddDays(-2)
                $fresh = New-Item -ItemType Directory -Path (Join-Path $workRoot 'reforge-89abcdef')
                $notOurs = New-Item -ItemType Directory -Path (Join-Path $workRoot 'reforge-notes')
                $notOurs.LastWriteTime = (Get-Date).AddDays(-2)
                InModuleScope Reforge -Parameters @{ W = $workRoot } { $script:SavedWorkRoot = $script:WorkRoot; $script:WorkRoot = $W }
            }
            AfterEach { InModuleScope Reforge { $script:WorkRoot = $script:SavedWorkRoot } }

            It 'deletes old ones from crashed runs, and only those' {
                Update-ReforgeApp -ConfigPath $config | Out-Null
                $stale.FullName | Should -Not -Exist
                $fresh.FullName | Should -Exist
                $notOurs.FullName | Should -Exist
            }

            It 'deletes nothing with -WhatIf' {
                Update-ReforgeApp -ConfigPath $config -WhatIf | Out-Null
                $stale.FullName | Should -Exist
            }
        }

        It 'reports a name that is not in the list' {
            Update-ReforgeApp 'Nope' -ConfigPath $config -ErrorAction SilentlyContinue -ErrorVariable err | Should -BeNullOrEmpty
            $err | Should -HaveCount 1
        }

        It 'warns when the list is empty' {
            Update-ReforgeApp -ConfigPath (New-TestConfigPath) -WarningVariable warnings 3>$null | Should -BeNullOrEmpty
            $warnings | Should -HaveCount 1
        }
    }
}
