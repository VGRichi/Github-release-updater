@{
    RootModule           = 'Reforge.psm1'
    ModuleVersion        = '3.0.0'
    GUID                 = '526e9794-727d-49de-be9d-b47a2fe67fc8'
    Author               = 'VGRichi'
    Copyright            = '(c) 2026 VGRichi. MIT License.'
    Description          = 'Keeps Windows apps and tools installed from GitHub releases up to date: .msi/.exe installers, and .zip/.7z/.tar.gz tools like DXVK. Verifies downloads against GitHub checksums and never installs an unverified file without asking.'
    PowerShellVersion    = '5.1'
    CompatiblePSEditions = @('Desktop', 'Core')
    HelpInfoURI          = 'https://github.com/VGRichi/Reforge/releases/latest/download/'
    FormatsToProcess     = @('Reforge.Format.ps1xml')
    FunctionsToExport    = @('Get-ReforgeApp', 'Add-ReforgeApp', 'Install-ReforgeApp', 'Remove-ReforgeApp', 'Find-ReforgeApp', 'Update-ReforgeApp')
    CmdletsToExport      = @()
    VariablesToExport    = @()
    AliasesToExport      = @()
    PrivateData          = @{
        PSData = @{
            Tags         = @('GitHub', 'Update', 'Updater', 'Releases', 'Winget', 'Installer', 'DXVK', 'Windows', 'PSEdition_Desktop', 'PSEdition_Core')
            LicenseUri   = 'https://github.com/VGRichi/Reforge/blob/main/LICENSE'
            ProjectUri   = 'https://github.com/VGRichi/Reforge'
            ReleaseNotes = 'https://github.com/VGRichi/Reforge/releases'
        }
    }
}
