Describe 'Archive helpers' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        Import-ReforgeForTest
    }
    AfterAll { $env:APPDATA = $originalAppData }

    BeforeEach {
        $work = Join-Path $TestDrive ([guid]::NewGuid().ToString('N').Substring(0, 8))
    }

    Context 'Expand-ReleaseArchive' {
        It 'extracts a .zip' {
            $src = New-TestTree (Join-Path $work 'src') @{ 'tool-1.0\tool.exe' = 'exe'; 'tool-1.0\lib\a.dll' = 'dll' }
            $zip = Join-Path $work 'tool.zip'
            Compress-Archive -Path (Join-Path $src 'tool-1.0') -DestinationPath $zip
            $out = Join-Path $work 'out'
            InModuleScope Reforge -Parameters @{ File = $zip; Out = $out } { Expand-ReleaseArchive $File $Out }
            Get-Content -LiteralPath (Join-Path $out 'tool-1.0\lib\a.dll') -Raw | Should -Be 'dll'
        }

        It 'extracts a .tar.gz with tar' -Skip:(-not (Get-Command tar -ErrorAction SilentlyContinue)) {
            $src = New-TestTree (Join-Path $work 'src') @{ 'dxvk-2.5\x64\d3d11.dll' = 'x64' }
            $tgz = Join-Path $work 'dxvk-2.5.tar.gz'
            & tar -czf $tgz -C $src 'dxvk-2.5'
            $out = Join-Path $work 'out'
            InModuleScope Reforge -Parameters @{ File = $tgz; Out = $out } { Expand-ReleaseArchive $File $Out }
            Get-Content -LiteralPath (Join-Path $out 'dxvk-2.5\x64\d3d11.dll') -Raw | Should -Be 'x64'
        }

        It 'throws when the archive is broken' -Skip:(-not (Get-Command tar -ErrorAction SilentlyContinue)) {
            New-Item -ItemType Directory -Path $work | Out-Null
            $bad = Join-Path $work 'broken.tar.gz'
            Set-Content -LiteralPath $bad -Value 'not an archive'
            { InModuleScope Reforge -Parameters @{ File = $bad; Out = (Join-Path $work 'out') } { Expand-ReleaseArchive $File $Out } } |
                Should -Throw '*Could not extract*'
        }
    }

    Context 'Get-ArchiveContentRoot' {
        It 'steps into a single wrapper folder' {
            $root = New-TestTree $work @{ 'dxvk-2.5\x64\d3d11.dll' = '1'; 'dxvk-2.5\x32\d3d11.dll' = '2' }
            InModuleScope Reforge -Parameters @{ Root = $root } { Get-ArchiveContentRoot $Root '' } | Should -Be (Join-Path $root 'dxvk-2.5')
        }

        It 'steps through nested wrapper folders' {
            $root = New-TestTree $work @{ 'a\b\tool.exe' = '1' }
            InModuleScope Reforge -Parameters @{ Root = $root } { Get-ArchiveContentRoot $Root '' } | Should -Be (Join-Path $root 'a\b')
        }

        It 'stays at the top when it holds files' {
            $root = New-TestTree $work @{ 'tool.exe' = '1' }
            InModuleScope Reforge -Parameters @{ Root = $root } { Get-ArchiveContentRoot $Root '' } | Should -Be $root
        }

        It 'returns -Subfolder inside the wrapper folder' {
            $root = New-TestTree $work @{ 'dxvk-2.5\x64\d3d11.dll' = '1'; 'dxvk-2.5\x32\d3d11.dll' = '2' }
            InModuleScope Reforge -Parameters @{ Root = $root } { Get-ArchiveContentRoot $Root 'x64' } | Should -Be (Join-Path $root 'dxvk-2.5\x64')
        }

        It 'accepts a multi-level -Subfolder' {
            $root = New-TestTree $work @{ 'pkg\readme.txt' = 'r'; 'pkg\release\x64\a.dll' = '1' }
            InModuleScope Reforge -Parameters @{ Root = $root } { Get-ArchiveContentRoot $Root 'release\x64' } | Should -Be (Join-Path $root 'pkg\release\x64')
        }

        It 'searches deeper for -Subfolder by its last folder name' {
            $root = New-TestTree $work @{ 'readme.txt' = 'r'; 'build\release\x64\a.dll' = '1' }
            InModuleScope Reforge -Parameters @{ Root = $root } { Get-ArchiveContentRoot $Root 'x64' } | Should -Be (Join-Path $root 'build\release\x64')
        }

        It 'throws when -Subfolder is not in the archive' {
            $root = New-TestTree $work @{ 'dxvk-2.5\x64\d3d11.dll' = '1' }
            { InModuleScope Reforge -Parameters @{ Root = $root } { Get-ArchiveContentRoot $Root 'arm64' } } | Should -Throw "*Folder 'arm64' was not found*"
        }
    }

    Context 'Copy-ArchiveContent' {
        BeforeEach {
            $source = New-TestTree (Join-Path $work 'source') @{
                'OptiScaler.dll'      = 'new dll'
                'OptiScaler.ini'      = 'new ini'
                'config\user.ini'     = 'new user'
                'docs\readme.txt'     = 'readme'
            }
            $dest = Join-Path $work 'dest'
            $backup = Join-Path $work 'backup'
            New-Item -ItemType Directory -Path $dest | Out-Null
        }

        It 'copies every file, keeping folders, and returns how many it copied' {
            $app = [pscustomobject]@{ Name = 'Tool' }
            $count = InModuleScope Reforge -Parameters @{ App = $app; S = $source; D = $dest; B = $backup } { Copy-ArchiveContent $App $S $D $B }
            $count | Should -Be 4
            Get-Content -LiteralPath (Join-Path $dest 'config\user.ini') -Raw | Should -Be 'new user'
            Test-Path -LiteralPath $backup | Should -BeFalse   # nothing was replaced
        }

        It 'only copies files matching -Include, by name or relative path' {
            $app = [pscustomobject]@{ Name = 'Tool'; Include = @('*.dll', 'docs\*') }
            InModuleScope Reforge -Parameters @{ App = $app; S = $source; D = $dest; B = $backup } { Copy-ArchiveContent $App $S $D $B } | Should -Be 2
            Test-Path -LiteralPath (Join-Path $dest 'OptiScaler.dll') | Should -BeTrue
            Test-Path -LiteralPath (Join-Path $dest 'docs\readme.txt') | Should -BeTrue
            Test-Path -LiteralPath (Join-Path $dest 'OptiScaler.ini') | Should -BeFalse
        }

        It 'renames files with a hashtable' {
            $app = [pscustomobject]@{ Name = 'Tool'; Rename = @{ 'OptiScaler.dll' = 'dxgi.dll' } }
            InModuleScope Reforge -Parameters @{ App = $app; S = $source; D = $dest; B = $backup } { Copy-ArchiveContent $App $S $D $B } | Out-Null
            Get-Content -LiteralPath (Join-Path $dest 'dxgi.dll') -Raw | Should -Be 'new dll'
            Test-Path -LiteralPath (Join-Path $dest 'OptiScaler.dll') | Should -BeFalse
        }

        It 'renames files with an object, as read back from apps.json' {
            $app = [pscustomobject]@{ Name = 'Tool'; Rename = [pscustomobject]@{ 'OptiScaler.dll' = 'dxgi.dll' } }
            InModuleScope Reforge -Parameters @{ App = $app; S = $source; D = $dest; B = $backup } { Copy-ArchiveContent $App $S $D $B } | Out-Null
            Test-Path -LiteralPath (Join-Path $dest 'dxgi.dll') | Should -BeTrue
        }

        It 'backs up a replaced file before overwriting it' {
            Set-Content -LiteralPath (Join-Path $dest 'OptiScaler.dll') -Value 'old dll' -NoNewline
            $app = [pscustomobject]@{ Name = 'Tool' }
            InModuleScope Reforge -Parameters @{ App = $app; S = $source; D = $dest; B = $backup } { Copy-ArchiveContent $App $S $D $B } | Out-Null
            Get-Content -LiteralPath (Join-Path $dest 'OptiScaler.dll') -Raw | Should -Be 'new dll'
            Get-Content -LiteralPath (Join-Path $backup 'OptiScaler.dll') -Raw | Should -Be 'old dll'
        }

        It 'never overwrites a -Keep file, but adds it when missing' {
            Set-Content -LiteralPath (Join-Path $dest 'OptiScaler.ini') -Value 'my settings' -NoNewline
            $app = [pscustomobject]@{ Name = 'Tool'; Keep = @('*.ini') }
            InModuleScope Reforge -Parameters @{ App = $app; S = $source; D = $dest; B = $backup } { Copy-ArchiveContent $App $S $D $B } | Out-Null
            Get-Content -LiteralPath (Join-Path $dest 'OptiScaler.ini') -Raw | Should -Be 'my settings'
            Get-Content -LiteralPath (Join-Path $dest 'config\user.ini') -Raw | Should -Be 'new user'
        }

        It 'matches -Keep against the renamed name too' {
            Set-Content -LiteralPath (Join-Path $dest 'dxgi.dll') -Value 'my dll' -NoNewline
            $app = [pscustomobject]@{ Name = 'Tool'; Rename = @{ 'OptiScaler.dll' = 'dxgi.dll' }; Keep = @('dxgi.dll') }
            InModuleScope Reforge -Parameters @{ App = $app; S = $source; D = $dest; B = $backup } { Copy-ArchiveContent $App $S $D $B } | Out-Null
            Get-Content -LiteralPath (Join-Path $dest 'dxgi.dll') -Raw | Should -Be 'my dll'
        }

        It 'matches -Keep against relative paths, like -Include does' -Tag KnownBug {
            New-Item -ItemType Directory -Path (Join-Path $dest 'config') | Out-Null
            Set-Content -LiteralPath (Join-Path $dest 'config\user.ini') -Value 'my user' -NoNewline
            $app = [pscustomobject]@{ Name = 'Tool'; Keep = @('config\*') }
            InModuleScope Reforge -Parameters @{ App = $app; S = $source; D = $dest; B = $backup } { Copy-ArchiveContent $App $S $D $B } | Out-Null
            Get-Content -LiteralPath (Join-Path $dest 'config\user.ini') -Raw | Should -Be 'my user'
        }
    }
}
