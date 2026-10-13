Describe 'Updating archive tools' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        $originalArch = $env:PROCESSOR_ARCHITECTURE
        $env:PROCESSOR_ARCHITECTURE = 'AMD64'
        Import-ReforgeForTest

        # A DXVK-like release: one wrapper folder with x64 and x32 subfolders
        $src = New-TestTree (Join-Path $TestDrive 'release-src') @{
            'dxvk-2.5\x64\d3d11.dll' = 'new d3d11 x64'
            'dxvk-2.5\x64\dxgi.dll'  = 'new dxgi x64'
            'dxvk-2.5\x32\d3d11.dll' = 'new d3d11 x32'
        }
        $zip = Join-Path $TestDrive 'dxvk-2.5.zip'
        Compress-Archive -Path (Join-Path $src 'dxvk-2.5') -DestinationPath $zip
        $zipDigest = Get-TestDigest ([IO.File]::ReadAllBytes($zip))
    }
    AfterAll {
        $env:APPDATA = $originalAppData
        $env:PROCESSOR_ARCHITECTURE = $originalArch
    }

    BeforeEach {
        $case = Join-Path $TestDrive ([guid]::NewGuid().ToString('N').Substring(0, 8))
        $config = Join-Path $case 'Reforge\apps.json'
        $game = New-TestTree (Join-Path $case 'Game\bin\x64') @{ 'd3d11.dll' = 'old d3d11'; 'game.exe' = 'game' }
        New-Item -ItemType Directory -Path (Split-Path -Parent $config) | Out-Null
        $app = [pscustomobject]@{ Name = 'DXVK (Test)'; Repository = 'doitsujin/dxvk'; Type = 'archive'; Destination = @($game); Subfolder = 'x64' }
        Write-TestAppList $config @($app)

        $release = New-TestRelease 'v2.5' -Assets (New-TestAsset 'dxvk-2.5.zip' $zipDigest)
        Mock -ModuleName Reforge Get-LatestRelease { $release }
        Mock -ModuleName Reforge Invoke-WebRequest { Copy-Item -LiteralPath $zip -Destination $OutFile }
        Mock -ModuleName Reforge Read-Choice { throw 'Unexpected prompt' }
    }

    It 'extracts the chosen subfolder into the destination' {
        $result = Update-ReforgeApp -ConfigPath $config
        $result.Status | Should -Be 'Updated'
        $result.Note | Should -Be 'Checksum verified'
        $result.Installed | Should -Be 'v2.5'
        Get-Content -LiteralPath (Join-Path $game 'd3d11.dll') -Raw | Should -Be 'new d3d11 x64'
        Get-Content -LiteralPath (Join-Path $game 'dxgi.dll') -Raw | Should -Be 'new dxgi x64'
        Get-Content -LiteralPath (Join-Path $game 'game.exe') -Raw | Should -Be 'game'
        Test-Path -LiteralPath (Join-Path $game 'x32') | Should -BeFalse
    }

    It 'records the installed version in the app list' {
        Update-ReforgeApp -ConfigPath $config | Out-Null
        $saved = Read-TestAppList $config
        $saved.InstalledVersion | Should -Be 'v2.5'
        $saved.HadDigest | Should -BeTrue
    }

    It 'backs up replaced files next to the app list' {
        Update-ReforgeApp -ConfigPath $config | Out-Null
        $backups = @(Get-ChildItem -LiteralPath (Join-Path $case 'Reforge\backups\DXVK (Test)') -Recurse -File)
        $backups | Should -HaveCount 1
        $backups[0].FullName | Should -BeLike '*\1\d3d11.dll'
        Get-Content -LiteralPath $backups[0].FullName -Raw | Should -Be 'old d3d11'
    }

    It 'keeps only the newest 3 backups for the app, and nothing else is touched' {
        $appBackups = Join-Path $case 'Reforge\backups\DXVK (Test)'
        foreach ($old in '2025-01-01_10-00-00', '2025-02-01_10-00-00', '2025-03-01_10-00-00', '2025-04-01_10-00-00', 'my notes') {
            New-Item -ItemType Directory -Path (Join-Path $appBackups $old) | Out-Null
        }
        Update-ReforgeApp -ConfigPath $config | Out-Null
        $left = @(Get-ChildItem -LiteralPath $appBackups -Directory | ForEach-Object Name | Sort-Object)
        $left | Should -HaveCount 4
        $left | Should -Contain 'my notes'
        $left | Should -Contain '2025-04-01_10-00-00'
        $left | Should -Contain '2025-03-01_10-00-00'
        $left | Should -Not -Contain '2025-02-01_10-00-00'
    }

    It 'keeps every backup when the update fails' {
        $appBackups = Join-Path $case 'Reforge\backups\DXVK (Test)'
        foreach ($old in '2025-01-01_10-00-00', '2025-02-01_10-00-00', '2025-03-01_10-00-00', '2025-04-01_10-00-00') {
            New-Item -ItemType Directory -Path (Join-Path $appBackups $old) | Out-Null
        }
        $app.Subfolder = 'arm64'
        Write-TestAppList $config @($app)
        Update-ReforgeApp -ConfigPath $config 3>$null | Out-Null
        Get-ChildItem -LiteralPath $appBackups -Directory | Should -HaveCount 4
    }

    It 'reports UpToDate on the next run, without downloading' {
        Update-ReforgeApp -ConfigPath $config | Out-Null
        (Update-ReforgeApp -ConfigPath $config).Status | Should -Be 'UpToDate'
        Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 1 -Exactly
    }

    It 'installs into every destination, with a backup folder for each' {
        $second = New-TestTree (Join-Path $case 'Other\bin') @{ 'd3d11.dll' = 'old other' }
        $app.Destination = @($game, $second)
        Write-TestAppList $config @($app)
        Update-ReforgeApp -ConfigPath $config | Out-Null
        Get-Content -LiteralPath (Join-Path $second 'd3d11.dll') -Raw | Should -Be 'new d3d11 x64'
        Get-ChildItem -LiteralPath (Join-Path $case 'Reforge\backups\DXVK (Test)') -Recurse -File |
            ForEach-Object { $_.Directory.Name } | Sort-Object | Should -Be @('1', '2')
    }

    It 'reads the old "Destinations" field' {
        $legacy = [pscustomobject]@{ Name = 'DXVK (Test)'; Repository = 'doitsujin/dxvk'; Type = 'archive'; Destinations = @($game); Subfolder = 'x64' }
        Write-TestAppList $config @($legacy)
        (Update-ReforgeApp -ConfigPath $config).Status | Should -Be 'Updated'
    }

    It 'only reports the update with -WhatIf' {
        $result = Update-ReforgeApp -ConfigPath $config -WhatIf
        $result.Status | Should -Be 'UpdateAvailable'
        Get-Content -LiteralPath (Join-Path $game 'd3d11.dll') -Raw | Should -Be 'old d3d11'
        Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 0 -Exactly
    }

    It 'skips when a destination folder is missing' {
        $app.Destination = @($game, (Join-Path $case 'Gone'))
        Write-TestAppList $config @($app)
        $result = Update-ReforgeApp -ConfigPath $config
        $result.Status | Should -Be 'Skipped'
        $result.Note | Should -BeLike 'Folder not found:*Gone'
        Get-Content -LiteralPath (Join-Path $game 'd3d11.dll') -Raw | Should -Be 'old d3d11'
    }

    It 'skips when no destination is set' {
        Write-TestAppList $config @([pscustomobject]@{ Name = 'DXVK (Test)'; Repository = 'doitsujin/dxvk'; Type = 'archive' })
        $result = Update-ReforgeApp -ConfigPath $config
        $result.Status | Should -Be 'Skipped'
        $result.Note | Should -Be 'No destination folder set'
    }

    It 'skips when the release has no archive' {
        $release = New-TestRelease 'v2.5' -Assets (New-TestAsset 'dxvk-setup.exe')
        $result = Update-ReforgeApp -ConfigPath $config
        $result.Status | Should -Be 'Skipped'
        $result.Note | Should -Be 'No zip/7z/tar.gz file in this release'
    }

    It 'fails, without recording the version, when -Subfolder is not in the archive' {
        $app.Subfolder = 'arm64'
        Write-TestAppList $config @($app)
        $result = Update-ReforgeApp -ConfigPath $config 3>$null
        $result.Status | Should -Be 'Failed'
        (Read-TestAppList $config).PSObject.Properties['InstalledVersion'] | Should -BeNullOrEmpty
    }

    Context 'when a file in the destination is in use' {
        BeforeEach {
            Set-Content -LiteralPath (Join-Path $game 'dxgi.dll') -Value 'old dxgi' -NoNewline
            $lock = [IO.File]::Open((Join-Path $game 'dxgi.dll'), 'Open', 'ReadWrite', 'None')
        }
        AfterEach { $lock.Dispose() }

        It 'reports Failed instead of Updated' {
            $result = Update-ReforgeApp -ConfigPath $config 3>$null
            $result.Status | Should -Be 'Failed'
            $result.Note | Should -BeLike "Couldn't update*is the app running?*put back."
        }

        It 'does not record the new version, so the next run tries again' {
            Update-ReforgeApp -ConfigPath $config 3>$null | Out-Null
            (Read-TestAppList $config).PSObject.Properties['InstalledVersion'] | Should -BeNullOrEmpty
        }

        It 'puts back the files it already replaced' {
            Update-ReforgeApp -ConfigPath $config 3>$null | Out-Null
            Get-Content -LiteralPath (Join-Path $game 'd3d11.dll') -Raw | Should -Be 'old d3d11'
        }

        It 'puts back files in destinations that were already done' {
            $first = New-TestTree (Join-Path $case 'First') @{ 'd3d11.dll' = 'old first' }
            $app.Destination = @($first, $game)
            Write-TestAppList $config @($app)
            Update-ReforgeApp -ConfigPath $config 3>$null | Out-Null
            Get-Content -LiteralPath (Join-Path $first 'd3d11.dll') -Raw | Should -Be 'old first'
            Test-Path -LiteralPath (Join-Path $first 'dxgi.dll') | Should -BeFalse   # added by the update, so removed again
        }
    }
}
