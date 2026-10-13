---
external help file: GitHubAppUpdater-help.xml
Module Name: GitHubAppUpdater
online version: https://github.com/VGRichi/github-app-updater/blob/main/docs/en-US/Remove-GitHubApp.md
schema: 2.0.0
---

# Remove-GitHubApp

## SYNOPSIS
Stops tracking an app or tool.

## SYNTAX

```
Remove-GitHubApp [-Name] <String[]> [-ConfigPath <String>] [-WhatIf]
 [-Confirm] [<CommonParameters>]
```

## DESCRIPTION
Removes apps from your list.
This does not uninstall the app or delete any files; it only
means Update-GitHubApp won't update it anymore.

## EXAMPLES

### EXAMPLE 1
```
Remove-GitHubApp -Name PhotoCraft
```

Stops tracking PhotoCraft.

### EXAMPLE 2
```
Get-GitHubApp -Name 'DXVK*' | Remove-GitHubApp -WhatIf
```

Shows which DXVK entries would be removed, without removing them.

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
The name(s) of the apps to remove.
Wildcards are supported.

```yaml
Type: String[]
Parameter Sets: (All)
Aliases:

Required: True
Position: 1
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: True
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

## NOTES

## RELATED LINKS
