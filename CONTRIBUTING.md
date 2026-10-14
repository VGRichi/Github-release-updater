# Contributing

## Tests

```powershell
.\build\invoke-tests.ps1
```

Installs Pester 5 for the current user if needed. Run it in both PowerShell 7 (`pwsh`) and Windows PowerShell 5.1 (`powershell`); the tests also run on every push and pull request, and before publishing. Tests tagged `KnownBug` describe bugs that aren't fixed yet; `.\build\invoke-tests.ps1 -KnownBugs` runs only those.

## Help

Help lives in `docs/<language>/*.md`. After editing it, run `.\build\build-help.ps1` to rebuild the help, and commit both. To add a translation, copy `docs/en-US` to a folder like `docs/az-AZ`, translate the text (not the parameter names or code), and run the script.

## Publishing a new version

1. Update `ModuleVersion` in `Reforge/Reforge.psd1`, then commit and push.
2. On GitHub, create a release with a matching tag (`v3.0.1` for version `3.0.1`).
3. A GitHub Action attaches `Reforge.zip` and the `Update-Help` files to the release, and publishes to the PowerShell Gallery if a repository secret named `PSGALLERY_API_KEY` is set.
