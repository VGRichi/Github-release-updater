Describe 'GitHub API helpers' {
    BeforeAll {
        . (Join-Path $PSScriptRoot 'TestHelpers.ps1')
        $originalAppData = $env:APPDATA
        $originalToken = $env:GITHUB_TOKEN
        Import-ReforgeForTest
    }
    AfterAll {
        $env:APPDATA = $originalAppData
        $env:GITHUB_TOKEN = $originalToken
    }

    Context 'Get-RequestHeader' {
        BeforeEach { $env:GITHUB_TOKEN = $null }

        It 'always sends a User-Agent and the GitHub JSON media type' {
            $headers = InModuleScope Reforge { Get-RequestHeader $null }
            $headers['User-Agent'] | Should -Be 'Reforge'
            $headers['Accept'] | Should -Be 'application/vnd.github+json'
            $headers.ContainsKey('Authorization') | Should -BeFalse
        }

        It 'uses -Token as a bearer token' {
            $token = ConvertTo-SecureString 'test-token' -AsPlainText -Force
            (InModuleScope Reforge -Parameters @{ Token = $token } { Get-RequestHeader $Token }).Authorization | Should -Be 'Bearer test-token'
        }

        It 'falls back to GITHUB_TOKEN' {
            $env:GITHUB_TOKEN = 'env-token'
            (InModuleScope Reforge { Get-RequestHeader $null }).Authorization | Should -Be 'Bearer env-token'
        }

        It 'prefers -Token over GITHUB_TOKEN' {
            $env:GITHUB_TOKEN = 'env-token'
            $token = ConvertTo-SecureString 'test-token' -AsPlainText -Force
            (InModuleScope Reforge -Parameters @{ Token = $token } { Get-RequestHeader $Token }).Authorization | Should -Be 'Bearer test-token'
        }
    }

    Context 'Get-LatestRelease' {
        BeforeAll {
            $headers = @{ 'User-Agent' = 'Reforge' }
            $releaseList = @(
                New-TestRelease 'v3.0.0' -Draft
                New-TestRelease 'v2.0.0-beta' -Prerelease
                New-TestRelease 'v1.0.0'
            )
        }

        It 'returns /releases/latest for stable releases' {
            Mock -ModuleName Reforge Invoke-RestMethod -ParameterFilter { $Uri -like '*/releases/latest' } { New-TestRelease 'v1.0.0' }
            Mock -ModuleName Reforge Invoke-RestMethod { throw 'Unexpected request' }
            (InModuleScope Reforge -Parameters @{ H = $headers } { Get-LatestRelease 'owner/repo' $false $H }).tag_name | Should -Be 'v1.0.0'
            Should -Invoke Invoke-RestMethod -ModuleName Reforge -Times 1 -Exactly -ParameterFilter { $Uri -eq 'https://api.github.com/repos/owner/repo/releases/latest' }
        }

        It 'falls back to the newest non-draft release, even a prerelease, when there is no stable release (404)' {
            Mock -ModuleName Reforge Invoke-RestMethod -ParameterFilter { $Uri -like '*/releases/latest' } { throw (New-HttpError 404) }
            Mock -ModuleName Reforge Invoke-RestMethod -ParameterFilter { $Uri -like '*/releases?per_page=*' } { $releaseList }
            (InModuleScope Reforge -Parameters @{ H = $headers } { Get-LatestRelease 'owner/repo' $false $H }).tag_name | Should -Be 'v2.0.0-beta'
        }

        It 'skips /releases/latest when prereleases are wanted' {
            Mock -ModuleName Reforge Invoke-RestMethod -ParameterFilter { $Uri -like '*/releases/latest' } { throw 'Should not be called' }
            Mock -ModuleName Reforge Invoke-RestMethod -ParameterFilter { $Uri -like '*/releases?per_page=*' } { $releaseList }
            (InModuleScope Reforge -Parameters @{ H = $headers } { Get-LatestRelease 'owner/repo' $true $H }).tag_name | Should -Be 'v2.0.0-beta'
        }

        It 'passes the request headers on' {
            Mock -ModuleName Reforge Invoke-RestMethod { New-TestRelease 'v1.0.0' }
            InModuleScope Reforge -Parameters @{ H = $headers } { Get-LatestRelease 'owner/repo' $false $H } | Out-Null
            Should -Invoke Invoke-RestMethod -ModuleName Reforge -ParameterFilter { $Headers['User-Agent'] -eq 'Reforge' }
        }

        It 'rethrows errors other than 404' {
            Mock -ModuleName Reforge Invoke-RestMethod { throw (New-HttpError 403) }
            { InModuleScope Reforge -Parameters @{ H = $headers } { Get-LatestRelease 'owner/repo' $false $H } } | Should -Throw '*403*'
        }

        It 'rethrows network errors that have no HTTP response' {
            Mock -ModuleName Reforge Invoke-RestMethod { throw 'No such host is known.' }
            { InModuleScope Reforge -Parameters @{ H = $headers } { Get-LatestRelease 'owner/repo' $false $H } } | Should -Throw '*No such host*'
        }

        It 'returns nothing when the repository has no releases' {
            Mock -ModuleName Reforge Invoke-RestMethod -ParameterFilter { $Uri -like '*/releases/latest' } { throw (New-HttpError 404) }
            Mock -ModuleName Reforge Invoke-RestMethod -ParameterFilter { $Uri -like '*/releases?per_page=*' } { @() }
            InModuleScope Reforge -Parameters @{ H = $headers } { Get-LatestRelease 'owner/repo' $false $H } | Should -BeNullOrEmpty
        }
    }
}
