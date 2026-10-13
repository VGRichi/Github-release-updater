---
external help file: Reforge-help.xml
Module Name: Reforge
online version: https://github.com/VGRichi/Reforge/blob/main/docs/en-US/Find-ReforgeApp.md
schema: 2.0.0
---

# Find-ReforgeApp

## SYNOPSIS
Finds installed apps that come from GitHub.

## SYNTAX

```
Find-ReforgeApp [[-ConfigPath] <String>] [<CommonParameters>]
```

## DESCRIPTION
Looks through Settings \> Apps \> Installed apps for apps whose uninstall information links to
a GitHub repository.
Apps already in your list and apps that winget can update are left out.
Nothing is added; pipe the results to Add-ReforgeApp to track them.

Apps that don't include a GitHub link in their uninstall information can't be found this way;
add those with Add-ReforgeApp.

## EXAMPLES

### EXAMPLE 1
```
Find-ReforgeApp
```

Shows installed apps that link to GitHub and aren't tracked yet.

### EXAMPLE 2
```
Find-ReforgeApp | Add-ReforgeApp
```

Tracks all of them.

### EXAMPLE 3
```
Find-ReforgeApp | Out-GridView -PassThru | Add-ReforgeApp
```

Lets you pick which ones to track in a window.

## PARAMETERS

### -ConfigPath
Use a different list file.

```yaml
Type: String
Parameter Sets: (All)
Aliases:

Required: False
Position: 1
Default value: $script:DefaultConfigPath
Accept pipeline input: False
Accept wildcard characters: False
```

### CommonParameters
This cmdlet supports the common parameters: -Debug, -ErrorAction, -ErrorVariable, -InformationAction, -InformationVariable, -OutVariable, -OutBuffer, -PipelineVariable, -Verbose, -WarningAction, and -WarningVariable. For more information, see [about_CommonParameters](http://go.microsoft.com/fwlink/?LinkID=113216).

## INPUTS

## OUTPUTS

### Reforge.Candidate
## NOTES

## RELATED LINKS
