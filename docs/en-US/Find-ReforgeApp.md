---
external help file: Reforge-help.xml
Module Name: Reforge
online version: https://github.com/VGRichi/Reforge/blob/main/docs/en-US/Find-ReforgeApp.md
schema: 2.0.0
---

# Find-ReforgeApp

## SYNOPSIS
Finds installed apps and downloaded tools that come from GitHub.

## SYNTAX

```
Find-ReforgeApp [[-Path] <String[]>] [-ConfigPath <String>] [<CommonParameters>]
```

## DESCRIPTION
Looks in two places:

Settings \> Apps \> Installed apps, for apps whose uninstall information links to a GitHub
repository.

Your Downloads folder (and two levels of subfolders), for files downloaded from GitHub releases.
Browsers note on each download the page it came from, which names the repository and release.
A downloaded installer is suggested when the app it installs is in Installed apps with the
same version.
A downloaded archive (.zip, .7z, .tar.gz) is suggested as an archive tool; Reforge can't know
which folder you extracted it into, so add it with -Destination.

Apps already in your list and apps that winget can update are left out.
Nothing is added; pipe the results to Add-ReforgeApp to track them.

Apps with no GitHub link and no download left in Downloads can't be found; add those with
Add-ReforgeApp.
Some tools remove the download note, like Unblock-File.

## EXAMPLES

### EXAMPLE 1
```
Find-ReforgeApp
```

Shows apps and tools from GitHub that aren't tracked yet.

### EXAMPLE 2
```
Find-ReforgeApp | Where-Object Type -eq Installer | Add-ReforgeApp
```

Tracks all the installed apps it found.

### EXAMPLE 3
```
Find-ReforgeApp | Out-GridView -PassThru | Add-ReforgeApp
```

Lets you pick which installed apps to track in a window.

### EXAMPLE 4
```
Find-ReforgeApp | Where-Object Name -eq OptiScaler | Add-ReforgeApp -Name 'OptiScaler (Cyberpunk)' -Destination 'D:\Games\Cyberpunk 2077\bin\x64'
```

Tracks a downloaded archive tool for the game folder you extracted it into.

### EXAMPLE 5
```
Find-ReforgeApp -Path D:\Mods, "$env:USERPROFILE\Desktop"
```

Looks for downloads in other folders instead of Downloads.

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

### -Path
Folders to look in for files downloaded from GitHub, with two levels of subfolders.
Defaults to your Downloads folder.

```yaml
Type: String[]
Parameter Sets: (All)
Aliases:

Required: False
Position: 1
Default value: Your Downloads folder
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
