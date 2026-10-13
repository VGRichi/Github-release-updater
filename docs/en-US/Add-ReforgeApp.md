---
external help file: Reforge-help.xml
Module Name: Reforge
online version: https://github.com/VGRichi/Reforge/blob/main/docs/en-US/Add-ReforgeApp.md
schema: 2.0.0
---

# Add-ReforgeApp

## SYNOPSIS
Starts tracking an app or tool from GitHub releases.

## SYNTAX

```
Add-ReforgeApp [-Repository] <String> [[-Name] <String>] [-Destination <String[]>] [-Subfolder <String>]
 [-Pattern <String>] [-Include <String[]>] [-Keep <String[]>] [-Rename <Object>] [-SilentArgs <String>]
 [-Prerelease] [-AllowUnverified] [-Force] [-PassThru] [-ConfigPath <String>]
 [<CommonParameters>]
```

## DESCRIPTION
Adds an app to your list so Update-ReforgeApp keeps it up to date.

Installer apps (.msi/.exe): give the repository and the app's name as it appears in
Settings \> Apps \> Installed apps (a partial name is fine).

Archive tools (.zip/.7z/.tar.gz), like DXVK or OptiScaler: also give -Destination, the folder
the files are extracted into.
Without -Destination, the app is treated as an installer app.

Accepts pipeline input by property name, so you can pipe objects, CSV rows or the output of
Find-ReforgeApp into it, or splat a hashtable.

## EXAMPLES

### EXAMPLE 1
```
Add-ReforgeApp -Repository storytold/photocraft -Name PhotoCraft
```

Tracks an app that was installed with an .msi or .exe.

### EXAMPLE 2
```
Add-ReforgeApp -Repository doitsujin/dxvk -Name 'DXVK (Witcher 3)' -Destination 'D:\Games\Witcher 3\bin\x64' -Subfolder x64
```

Tracks DXVK for one game, copying the archive's x64 folder into the game folder.

### EXAMPLE 3
```
Add-ReforgeApp -Repository optiscaler/OptiScaler -Name 'OptiScaler (Cyberpunk)' -Destination 'D:\Games\Cyberpunk 2077\bin\x64' -Rename @{ 'OptiScaler.dll' = 'dxgi.dll' } -Keep '*.ini'
```

Tracks OptiScaler, renaming its DLL and never overwriting your settings file.

### EXAMPLE 4
```
$app = @{ Repository = 'storytold/pdfcraft'; Name = 'PdfCraft' }
Add-ReforgeApp @app
```

Adds an app from a hashtable (splatting).

### EXAMPLE 5
```
Import-Csv .\my-apps.csv | Add-ReforgeApp
```

Adds every row of a CSV file with Repository and Name columns.

### EXAMPLE 6
```
Find-ReforgeApp | Add-ReforgeApp
```

Adds every installed app that Find-ReforgeApp found.

## PARAMETERS

### -AllowUnverified
Trust this app, so releases without a GitHub checksum install without asking.
This never skips
the warning when an app whose earlier releases had checksums suddenly doesn't.

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

### -Destination
Makes this an archive tool and sets the folder(s) its files are extracted into.

```yaml
Type: String[]
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -Force
Replace an app that's already in the list under the same name.

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

### -Include
Archive tools: only copy files matching these wildcards, for example '*.dll'.
Matches file names or paths inside the archive ('bin\*').

```yaml
Type: String[]
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -Keep
Archive tools: files that are added if missing but never overwritten, like your settings
files ('*.ini').
Matches file names or paths inside the archive ('config\*').

```yaml
Type: String[]
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -Name
A name for the app.
For installer apps, it must match the name in Installed apps.
Defaults to
the repository name.
Each name in your list must be unique.

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

### -PassThru
Output the added app.

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

### -Pattern
Which release file to download, as a wildcard, for example '*win-x64.msi'.
Only needed if the
automatic choice is wrong.

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
Follow pre-releases (betas) instead of only stable releases.

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

### -Rename
Archive tools: rename files while copying, as a hashtable, for example
@{ 'OptiScaler.dll' = 'dxgi.dll' }.

```yaml
Type: Object
Parameter Sets: (All)
Aliases:

Required: False
Position: Named
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -Repository
The GitHub repository as owner/repo, or its full GitHub link.

```yaml
Type: String
Parameter Sets: (All)
Aliases: Repo

Required: True
Position: 1
Default value: None
Accept pipeline input: True (ByPropertyName)
Accept wildcard characters: False
```

### -SilentArgs
Installer apps: silent-install arguments, if the installer type isn't detected automatically.

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

### -Subfolder
Archive tools: the folder inside the archive to copy from, for example x64 for DXVK.

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

### CommonParameters
This cmdlet supports the common parameters: -Debug, -ErrorAction, -ErrorVariable, -InformationAction, -InformationVariable, -OutVariable, -OutBuffer, -PipelineVariable, -Verbose, -WarningAction, and -WarningVariable. For more information, see [about_CommonParameters](http://go.microsoft.com/fwlink/?LinkID=113216).

## INPUTS

## OUTPUTS

### Reforge.App
## NOTES

## RELATED LINKS
