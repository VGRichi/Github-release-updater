---
external help file: GitHubAppUpdater-help.xml
Module Name: GitHubAppUpdater
online version: https://github.com/VGRichi/github-app-updater/blob/main/docs/en-US/Update-GitHubApp.md
schema: 2.0.0
---

# Update-GitHubApp

## SYNOPSIS
Updates your tracked apps and tools to their latest GitHub release.

## SYNTAX

```
Update-GitHubApp [[-Name] <String[]>] [-Token <SecureString>] [-ConfigPath <String>]
 [-WhatIf] [-Confirm] [<CommonParameters>]
```

## DESCRIPTION
Checks each app in your list against its latest GitHub release and installs newer versions.

Installer apps (.msi/.exe) are compared with the version in Installed apps and installed
silently.
Apps installed for all users need PowerShell running as administrator; apps
installed only for your account don't.
Archive tools are extracted into their folders,
with replaced files backed up first.

Downloads with a GitHub checksum are verified before installing.
Downloads without one are
only installed after you confirm, or if the app is trusted (Add-GitHubApp -AllowUnverified).
If an app's earlier releases had checksums and a new one doesn't, it always asks.

Use -WhatIf to see what would be updated without installing anything (no admin needed).
Outputs one result per app.

## EXAMPLES

### EXAMPLE 1
```
Update-GitHubApp -WhatIf
```

Shows which apps have updates, without installing anything.

### EXAMPLE 2
```
Update-GitHubApp
```

Updates everything.
Apps installed for all users need PowerShell running as administrator.

### EXAMPLE 3
```
Update-GitHubApp -Name 'DXVK*' -Verbose
```

Updates only the DXVK entries and shows each step.

### EXAMPLE 4
```
Update-GitHubApp -Token (Read-Host -AsSecureString 'GitHub token')
```

Uses a GitHub token for this run.

### EXAMPLE 5
```
Update-GitHubApp | Where-Object Status -eq 'Failed'
```

Shows only the apps that failed to update.

## PARAMETERS

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

### -Name
Only update apps whose name matches.
Wildcards are supported.
Defaults to all apps.

```yaml
Type: String[]
Parameter Sets: (All)
Aliases:

Required: False
Position: 1
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: True
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

### GitHubAppUpdater.Result
## NOTES

## RELATED LINKS
