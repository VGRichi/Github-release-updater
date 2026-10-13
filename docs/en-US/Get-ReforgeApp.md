---
external help file: Reforge-help.xml
Module Name: Reforge
online version: https://github.com/VGRichi/Reforge/blob/main/docs/en-US/Get-ReforgeApp.md
schema: 2.0.0
---

# Get-ReforgeApp

## SYNOPSIS
Lists the apps and tools you're tracking.

## SYNTAX

```
Get-ReforgeApp [[-Name] <String[]>] [-ConfigPath <String>]
 [<CommonParameters>]
```

## DESCRIPTION
Shows every app in your list, with its GitHub repository, whether it's an installer app
(.msi/.exe) or an archive tool (.zip/.7z/.tar.gz), and where archive tools are extracted.

## EXAMPLES

### EXAMPLE 1
```
Get-ReforgeApp
```

Lists everything you're tracking.

### EXAMPLE 2
```
Get-ReforgeApp -Name DXVK*
```

Lists only the apps whose name starts with DXVK.

### EXAMPLE 3
```
Get-ReforgeApp | Format-List *
```

Shows every detail, including installed versions of archive tools and which apps are trusted.

## PARAMETERS

### -ConfigPath
Use a different list file, for example to test without touching your real list.

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
Only show apps whose name matches.
Wildcards are supported.

```yaml
Type: String[]
Parameter Sets: (All)
Aliases:

Required: False
Position: 1
Default value: *
Accept pipeline input: False
Accept wildcard characters: True
```

### CommonParameters
This cmdlet supports the common parameters: -Debug, -ErrorAction, -ErrorVariable, -InformationAction, -InformationVariable, -OutVariable, -OutBuffer, -PipelineVariable, -Verbose, -WarningAction, and -WarningVariable. For more information, see [about_CommonParameters](http://go.microsoft.com/fwlink/?LinkID=113216).

## INPUTS

## OUTPUTS

### Reforge.App
## NOTES

## RELATED LINKS
