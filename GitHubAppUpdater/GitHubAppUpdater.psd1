@{
    RootModule           = 'GitHubAppUpdater.psm1'
    ModuleVersion        = '2.0.0'
    GUID                 = '34fd814f-f924-40f2-a6b4-f71ed3271a7a'
    Author               = 'VGRichi'
    Copyright            = '(c) 2026 VGRichi. MIT License.'
    Description          = 'Keeps Windows apps and tools installed from GitHub releases up to date: .msi/.exe installers, and .zip/.7z/.tar.gz tools like DXVK. Verifies downloads against GitHub checksums and never installs an unverified file without asking.'
    PowerShellVersion    = '5.1'
    CompatiblePSEditions = @('Desktop', 'Core')
    HelpInfoURI          = 'https://github.com/VGRichi/github-app-updater/releases/latest/download/'
    FormatsToProcess     = @('GitHubAppUpdater.Format.ps1xml')
    FunctionsToExport    = @('Get-GitHubApp', 'Add-GitHubApp', 'Remove-GitHubApp', 'Find-GitHubApp', 'Update-GitHubApp')
    CmdletsToExport      = @()
    VariablesToExport    = @()
    AliasesToExport      = @()
    PrivateData          = @{
        PSData = @{
            Tags         = @('GitHub', 'Update', 'Updater', 'Releases', 'Winget', 'Installer', 'DXVK', 'Windows', 'PSEdition_Desktop', 'PSEdition_Core')
            LicenseUri   = 'https://github.com/VGRichi/github-app-updater/blob/main/LICENSE'
            ProjectUri   = 'https://github.com/VGRichi/github-app-updater'
            ReleaseNotes = 'https://github.com/VGRichi/github-app-updater/releases'
        }
    }
}
