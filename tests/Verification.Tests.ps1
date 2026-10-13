Describe 'Download verification' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        Import-ReforgeForTest
    }
    AfterAll { $env:APPDATA = $originalAppData }

    Context 'Get-VerificationNote' {
        It 'says "<Expected>" when digest=<HasDigest>, HadDigest=<HadDigest>, trusted=<Trusted>' -TestCases @(
            @{ HasDigest = $true; HadDigest = $false; Trusted = $false; Expected = 'Checksum available' }
            @{ HasDigest = $false; HadDigest = $true; Trusted = $true; Expected = 'No checksum (unusual: earlier releases had one)' }
            @{ HasDigest = $false; HadDigest = $false; Trusted = $true; Expected = 'No checksum (trusted app)' }
            @{ HasDigest = $false; HadDigest = $false; Trusted = $false; Expected = 'No checksum (will ask before installing)' }
        ) {
            $app = [pscustomobject]@{ Name = 'App'; HadDigest = $HadDigest; AllowUnverified = $Trusted }
            $asset = New-TestAsset 'App.exe' $(if ($HasDigest) { 'sha256:00' })
            InModuleScope Reforge -Parameters @{ App = $app; Asset = $asset } { Get-VerificationNote $App $Asset } | Should -Be $Expected
        }
    }

    Context 'Approve-Install' {
        BeforeEach {
            Mock -ModuleName Reforge Read-Choice { throw 'Unexpected prompt' }
            InModuleScope Reforge { $script:ListChanged = $false }
        }

        It 'approves a download with a checksum without asking' {
            $app = [pscustomobject]@{ Name = 'App' }
            $asset = New-TestAsset 'App.exe' 'sha256:00'
            InModuleScope Reforge -Parameters @{ App = $app; Asset = $asset } { Approve-Install $App $Asset } | Should -BeTrue
            Should -Invoke Read-Choice -ModuleName Reforge -Times 0 -Exactly
        }

        It 'approves a trusted app without asking' {
            $app = [pscustomobject]@{ Name = 'App'; AllowUnverified = $true }
            InModuleScope Reforge -Parameters @{ App = $app; Asset = (New-TestAsset 'App.exe') } { Approve-Install $App $Asset } | Should -BeTrue
            Should -Invoke Read-Choice -ModuleName Reforge -Times 0 -Exactly
        }

        It 'asks Yes/Always/No, defaulting to No, when there is no checksum' {
            Mock -ModuleName Reforge Read-Choice { 2 }
            $app = [pscustomobject]@{ Name = 'App' }
            InModuleScope Reforge -Parameters @{ App = $app; Asset = (New-TestAsset 'App.exe') } { Approve-Install $App $Asset } | Should -BeFalse
            Should -Invoke Read-Choice -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { $Labels.Count -eq 3 -and $Default -eq 2 }
        }

        It 'approves once on Yes, without trusting the app' {
            Mock -ModuleName Reforge Read-Choice { 0 }
            $app = [pscustomobject]@{ Name = 'App' }
            InModuleScope Reforge -Parameters @{ App = $app; Asset = (New-TestAsset 'App.exe') } { Approve-Install $App $Asset } | Should -BeTrue
            $app.PSObject.Properties['AllowUnverified'] | Should -BeNullOrEmpty
        }

        It 'trusts the app from now on when the answer is Always' {
            Mock -ModuleName Reforge Read-Choice { 1 }
            $app = [pscustomobject]@{ Name = 'App' }
            InModuleScope Reforge -Parameters @{ App = $app; Asset = (New-TestAsset 'App.exe') } { Approve-Install $App $Asset } | Should -BeTrue
            $app.AllowUnverified | Should -BeTrue
            InModuleScope Reforge { $script:ListChanged } | Should -BeTrue
        }

        It 'always asks Yes/No (default No) when earlier releases had a checksum, even for a trusted app' {
            Mock -ModuleName Reforge Read-Choice { 1 }
            $app = [pscustomobject]@{ Name = 'App'; HadDigest = $true; AllowUnverified = $true }
            InModuleScope Reforge -Parameters @{ App = $app; Asset = (New-TestAsset 'App.exe') } { Approve-Install $App $Asset 3>$null } | Should -BeFalse
            Should -Invoke Read-Choice -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { $Labels.Count -eq 2 -and $Default -eq 1 }
        }
    }

    Context 'Save-ReleaseAsset' {
        BeforeAll {
            $payload = [Text.Encoding]::ASCII.GetBytes('installer bytes')
            Mock -ModuleName Reforge Invoke-WebRequest { [IO.File]::WriteAllBytes($OutFile, [Text.Encoding]::ASCII.GetBytes('installer bytes')) }
        }
        BeforeEach {
            $folder = Join-Path $TestDrive ([guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path $folder | Out-Null
        }

        It 'downloads the file and keeps it when the checksum matches' {
            $asset = New-TestAsset 'App.exe' (Get-TestDigest $payload)
            $file = InModuleScope Reforge -Parameters @{ Asset = $asset; Folder = $folder } { Save-ReleaseAsset $Asset $Folder }
            $file | Should -Be (Join-Path $folder 'App.exe')
            Test-Path -LiteralPath $file | Should -BeTrue
        }

        It 'deletes the file and throws when the checksum does not match' {
            $asset = New-TestAsset 'App.exe' (Get-TestDigest ([Text.Encoding]::ASCII.GetBytes('something else')))
            { InModuleScope Reforge -Parameters @{ Asset = $asset; Folder = $folder } { Save-ReleaseAsset $Asset $Folder } } |
                Should -Throw '*Checksum mismatch*'
            Test-Path -LiteralPath (Join-Path $folder 'App.exe') | Should -BeFalse
        }

        It 'keeps a file without a checksum (the approval step decides about it)' {
            $file = InModuleScope Reforge -Parameters @{ Asset = (New-TestAsset 'App.exe'); Folder = $folder } { Save-ReleaseAsset $Asset $Folder }
            Test-Path -LiteralPath $file | Should -BeTrue
        }

        It 'downloads without sending the GitHub token' {
            $savedToken = $env:GITHUB_TOKEN
            $env:GITHUB_TOKEN = 'test-token'
            try {
                InModuleScope Reforge -Parameters @{ Asset = (New-TestAsset 'App.exe'); Folder = $folder } { Save-ReleaseAsset $Asset $Folder } | Out-Null
            } finally { $env:GITHUB_TOKEN = $savedToken }
            Should -Invoke Invoke-WebRequest -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { -not $Headers.ContainsKey('Authorization') }
        }
    }
}
