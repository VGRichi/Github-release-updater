---
external help file: Reforge-help.xml
Module Name: Reforge
online version: https://github.com/VGRichi/Reforge/blob/main/docs/en-US/Install-ReforgeApp.md
schema: 2.0.0
---

# Install-ReforgeApp

## SYNOPSIS
Installs an app from its latest GitHub release and starts tracking it.

## SYNTAX

```
Install-ReforgeApp [-Repository] <String> [[-Name] <String>] [-Pattern <String>] [-SilentArgs <String>]
 [-Prerelease] [-AllowUnverified] [-Token <SecureString>] [-Force] [-ConfigPath <String>] [-WhatIf]
 [-Confirm] [<CommonParameters>]
```

## DESCRIPTION
Downloads the Windows installer (.msi or .exe) from a repository's latest release, installs it
silently, and adds the app to your list so Update-ReforgeApp keeps it up to date.
When a release has both, the .msi is used.

It always asks before installing, showing the file and whether it has a GitHub checksum.
A download without a checksum gets the same question as in Update-ReforgeApp (Yes, Always, No).

The app is tracked by the name it gets in Installed apps, which Install-ReforgeApp sees appear
during the install.
Give -Name to choose the name yourself.
If no new app appears (for example, it was already installed), it isn't tracked; track it with
Add-ReforgeApp and the name shown in Installed apps.

Some installers need PowerShell running as administrator.
For archive tools like DXVK, use Add-ReforgeApp -Destination, then Update-ReforgeApp.

## EXAMPLES

### EXAMPLE 1
```
Install-ReforgeApp storytold/photocraft
```

Installs PhotoCraft from its latest release, after asking, and starts tracking it.

### EXAMPLE 2
```
Install-ReforgeApp https://github.com/storytold/photocraft -WhatIf
```

Shows which file would be installed and whether it has a checksum, without installing anything.

### EXAMPLE 3
```
Install-ReforgeApp storytold/photocraft -Pattern '*setup.exe' -Force
```

Installs the setup .exe instead of the .msi, without asking first (a download without a checksum
is still asked about).

## PARAMETERS

### -AllowUnverified
Trust this app: install releases that have no GitHub checksum without asking about them.

```yaml
Type: SwitchParameter
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: False
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -ConfigPath
Use a different list file.

```yaml
Type: String
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: $script:DefaultConfigPath
Accept pipeline input: False
Accept wildcard characters: False
```

### -Force
Don't ask before installing.
Downloads without a checksum are still asked about, unless the app is trusted (-AllowUnverified).

```yaml
Type: SwitchParameter
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: False
Accept pipeline input: False
Accept wildcard characters: False
```

### -Name
The name to track the app by, as shown in Settings \> Apps \> Installed apps (a partial name is
fine).
Defaults to the name the app gets when it's installed.

```yaml
Type: String
Parameter Sets: (All)
Aliases:

Required: False
Position: 2
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -Pattern
Pick the release file yourself, with a wildcard like '*win-x64.msi', if the automatic choice is
wrong.

```yaml
Type: String
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -Prerelease
Install and follow beta releases.

```yaml
Type: SwitchParameter
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: False
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -Repository
The GitHub repository as owner/repo, or its full GitHub link.

```yaml
Type: String
Parameter Sets: (All)
Aliases:

Required: True
Position: 1
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -SilentArgs
Silent-install arguments for an .exe installer type that isn't detected automatically.

```yaml
Type: String
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -Token
A GitHub personal access token, to raise GitHub's limit of about 60 checks per hour.
Without it, the GITHUB_TOKEN environment variable is used if set.

```yaml
Type: SecureString
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: None
Accept pipeline input: False
Accept wildcard characters: False
```

### -Confirm
Prompts you for confirmation before running the cmdlet.

```yaml
Type: SwitchParameter
Parameter Sets: (All)
Aliases: cf

Required: False
Position: Named
Default value: None
Accept pipeline input: False
Accept wildcard characters: False
```

### -WhatIf
Shows what would happen if the cmdlet runs.
The cmdlet is not run.

```yaml
Type: SwitchParameter
Parameter Sets: (All)
Aliases: wi

Required: False
Position: Named
Default value: None
Accept pipeline input: False
Accept wildcard characters: False
```

### CommonParameters
This cmdlet supports the common parameters: -Debug, -ErrorAction, -ErrorVariable, -InformationAction, -InformationVariable, -OutVariable, -OutBuffer, -PipelineVariable, -Verbose, -WarningAction, and -WarningVariable. For more information, see [about_CommonParameters](http://go.microsoft.com/fwlink/?LinkID=113216).

## INPUTS

## OUTPUTS

### Reforge.Result
## NOTES

## RELATED LINKS
