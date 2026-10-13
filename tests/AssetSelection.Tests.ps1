Describe 'Asset selection and versions' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        $originalArch = $env:PROCESSOR_ARCHITECTURE
        Import-ReforgeForTest
    }
    AfterAll {
        $env:APPDATA = $originalAppData
        $env:PROCESSOR_ARCHITECTURE = $originalArch
    }

    Context 'ConvertTo-AppVersion' {
        It 'reads <Text> as <Expected>' -TestCases @(
            @{ Text = 'v1.2.3'; Expected = '1.2.3' }
            @{ Text = '1.2.3.4'; Expected = '1.2.3.4' }
            @{ Text = '2'; Expected = '2.0' }
            @{ Text = 'release-2024.10'; Expected = '2024.10' }
            @{ Text = '1.2.3.4.5'; Expected = '1.2.3.4' }
            @{ Text = '1.2.0-beta3'; Expected = '1.2.0' }
            @{ Text = 'App2-v1.3'; Expected = '2.0' }   # takes the first number, even inside a name
        ) {
            InModuleScope Reforge -Parameters @{ Text = $Text } { ConvertTo-AppVersion $Text } | Should -Be ([version]$Expected)
        }

        It 'returns nothing for <Text>' -TestCases @(
            @{ Text = 'nightly' }
            @{ Text = '' }
        ) {
            InModuleScope Reforge -Parameters @{ Text = $Text } { ConvertTo-AppVersion $Text } | Should -BeNullOrEmpty
        }

        It 'compares numerically, so 1.10 is newer than 1.9' {
            InModuleScope Reforge { (ConvertTo-AppVersion 'v1.10.0') -gt (ConvertTo-AppVersion 'v1.9.0') } | Should -BeTrue
        }
    }

    Context 'Select-InstallerAsset on an x64 PC' {
        BeforeAll { $env:PROCESSOR_ARCHITECTURE = 'AMD64' }

        It 'picks the x64 .msi when the app was installed with an .msi' {
            $release = New-TestRelease -Assets (New-TestAsset 'App-1.1-arm64.msi'), (New-TestAsset 'App-1.1-x64.msi'), (New-TestAsset 'App-1.1-x64-setup.exe')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-InstallerAsset $Release '' $true).name } | Should -Be 'App-1.1-x64.msi'
        }

        It 'picks the x64 .exe when the app was installed with an .exe' {
            $release = New-TestRelease -Assets (New-TestAsset 'App-1.1-arm64.msi'), (New-TestAsset 'App-1.1-x64.msi'), (New-TestAsset 'App-1.1-x64-setup.exe')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-InstallerAsset $Release '' $false).name } | Should -Be 'App-1.1-x64-setup.exe'
        }

        It 'treats x86_64 as x64' {
            $release = New-TestRelease -Assets (New-TestAsset 'App_x86_64.exe')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-InstallerAsset $Release '' $false).name } | Should -Be 'App_x86_64.exe'
        }

        It 'accepts an installer whose name has no architecture' {
            $release = New-TestRelease -Assets (New-TestAsset 'App-Setup.exe')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-InstallerAsset $Release '' $false).name } | Should -Be 'App-Setup.exe'
        }

        It 'never picks an installer for another architecture' {
            $release = New-TestRelease -Assets (New-TestAsset 'App-arm64.exe'), (New-TestAsset 'App-x86.exe'), (New-TestAsset 'App-ia32.msi'), (New-TestAsset 'App-32bit.exe')
            InModuleScope Reforge -Parameters @{ Release = $release } { Select-InstallerAsset $Release '' $false } | Should -BeNullOrEmpty
        }

        It 'ignores portable builds, other platforms and non-installers' {
            $release = New-TestRelease -Assets (New-TestAsset 'App-portable-x64.exe'), (New-TestAsset 'App-x64.dmg'), (New-TestAsset 'App-linux-x64.exe'), (New-TestAsset 'App-x64.zip')
            InModuleScope Reforge -Parameters @{ Release = $release } { Select-InstallerAsset $Release '' $false } | Should -BeNullOrEmpty
        }

        It 'uses -Pattern instead of the automatic choice' {
            $release = New-TestRelease -Assets (New-TestAsset 'App-x64.msi'), (New-TestAsset 'App-arm64.msi')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-InstallerAsset $Release '*arm64.msi' $true).name } | Should -Be 'App-arm64.msi'
        }

        It 'does not mistake names containing "mac" for macOS builds' {
            $release = New-TestRelease -Assets (New-TestAsset 'emacs-29.4-installer.exe')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-InstallerAsset $Release '' $false).name } | Should -Be 'emacs-29.4-installer.exe'
        }

        It 'still ignores <Name>' -TestCases @(
            @{ Name = 'App-macos-setup.exe' }, @{ Name = 'App_osx.exe' }, @{ Name = 'App-linux64.exe' }, @{ Name = 'App.Portable.exe' }
        ) {
            $release = New-TestRelease -Assets (New-TestAsset $Name)
            InModuleScope Reforge -Parameters @{ Release = $release } { Select-InstallerAsset $Release '' $false } | Should -BeNullOrEmpty
        }
    }

    Context 'Select-InstallerAsset on an ARM64 PC' {
        BeforeAll { $env:PROCESSOR_ARCHITECTURE = 'ARM64' }

        It 'prefers the arm64 installer' {
            $release = New-TestRelease -Assets (New-TestAsset 'App-x64.exe'), (New-TestAsset 'App-arm64.exe')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-InstallerAsset $Release '' $false).name } | Should -Be 'App-arm64.exe'
        }

        It 'skips an x64-only release (no emulation fallback)' {
            $release = New-TestRelease -Assets (New-TestAsset 'App-x64.exe')
            InModuleScope Reforge -Parameters @{ Release = $release } { Select-InstallerAsset $Release '' $false } | Should -BeNullOrEmpty
        }
    }

    Context 'Select-ArchiveAsset on an x64 PC' {
        BeforeAll { $env:PROCESSOR_ARCHITECTURE = 'AMD64' }

        It 'prefers the Windows x64 archive over source, Linux and generic ones' {
            $release = New-TestRelease -Assets (New-TestAsset 'tool-1.0.tar.gz'), (New-TestAsset 'tool-src.zip'), (New-TestAsset 'tool-linux-x64.tar.gz'), (New-TestAsset 'tool-win-x64.zip')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-ArchiveAsset $Release '').name } | Should -Be 'tool-win-x64.zip'
        }

        It 'picks the DXVK archive, not dxvk-native' {
            $release = New-TestRelease -Assets (New-TestAsset 'dxvk-native-2.5-steamrt-sniper.tar.gz'), (New-TestAsset 'dxvk-2.5.tar.gz')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-ArchiveAsset $Release '').name } | Should -Be 'dxvk-2.5.tar.gz'
        }

        It 'accepts <Name>' -TestCases @(
            @{ Name = 'tool.zip' }, @{ Name = 'tool.7z' }, @{ Name = 'tool.tar.gz' }, @{ Name = 'tool.tgz' }, @{ Name = 'tool.tar.xz' }
        ) {
            $release = New-TestRelease -Assets (New-TestAsset $Name)
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-ArchiveAsset $Release '').name } | Should -Be $Name
        }

        It 'ignores unsupported formats and debug symbols' {
            $release = New-TestRelease -Assets (New-TestAsset 'tool.rar'), (New-TestAsset 'tool-win-x64-pdb.zip'), (New-TestAsset 'tool-debug.zip')
            InModuleScope Reforge -Parameters @{ Release = $release } { Select-ArchiveAsset $Release '' } | Should -BeNullOrEmpty
        }

        It 'uses -Pattern instead of the automatic choice' {
            $release = New-TestRelease -Assets (New-TestAsset 'tool-win-x64.zip'), (New-TestAsset 'tool-full.7z')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-ArchiveAsset $Release '*full.7z').name } | Should -Be 'tool-full.7z'
        }

        It 'does not pick the arm64 archive when a generic Windows one exists' {
            $release = New-TestRelease -Assets (New-TestAsset 'tool-win-arm64.zip'), (New-TestAsset 'tool-win.zip')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-ArchiveAsset $Release '').name } | Should -Be 'tool-win.zip'
        }

        It 'picks nothing rather than an archive for another architecture' {
            $release = New-TestRelease -Assets (New-TestAsset 'tool-win-arm64.zip'), (New-TestAsset 'tool-win-x86.zip')
            InModuleScope Reforge -Parameters @{ Release = $release } { Select-ArchiveAsset $Release '' } | Should -BeNullOrEmpty
        }

        It 'treats win32 as a Windows name, not 32-bit' {
            $release = New-TestRelease -Assets (New-TestAsset 'tool-win32.zip')
            InModuleScope Reforge -Parameters @{ Release = $release } { (Select-ArchiveAsset $Release '').name } | Should -Be 'tool-win32.zip'
        }

        It 'does not take "Emacs" or "Macro" for macOS builds' {
            $release = New-TestRelease -Assets (New-TestAsset 'emacs-29-win64.zip'), (New-TestAsset 'MacroTool-win64.zip')
            InModuleScope Reforge -Parameters @{ Release = $release } { Select-ArchiveAsset $Release '' } | Should -Not -BeNullOrEmpty
        }
    }
}
