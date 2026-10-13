---
external help file: GitHubAppUpdater-help.xml
Module Name: GitHubAppUpdater
online version: https://github.com/VGRichi/github-app-updater/blob/main/docs/en-US/Find-GitHubApp.md
schema: 2.0.0
---

# Find-GitHubApp

## SYNOPSIS
Finds installed apps that come from GitHub.

## SYNTAX

```
Find-GitHubApp [[-ConfigPath] <String>] [<CommonParameters>]
```

## DESCRIPTION
Looks through Settings \> Apps \> Installed apps for apps whose uninstall information links to
a GitHub repository.
Apps already in your list and apps that winget can update are left out.
Nothing is added; pipe the results to Add-GitHubApp to track them.

Apps that don't include a GitHub link in their uninstall information can't be found this way;
add those with Add-GitHubApp.

## EXAMPLES

### EXAMPLE 1
```
Find-GitHubApp
```

Shows installed apps that link to GitHub and aren't tracked yet.

### EXAMPLE 2
```
Find-GitHubApp | Add-GitHubApp
```

Tracks all of them.

### EXAMPLE 3
```
Find-GitHubApp | Out-GridView -PassThru | Add-GitHubApp
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

### GitHubAppUpdater.Candidate
## NOTES

## RELATED LINKS
