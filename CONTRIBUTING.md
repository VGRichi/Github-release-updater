# Contributing

Reforge is written in Go. [docs/design.md](docs/design.md) explains how it's put together.

## Build and test

```
go build ./cmd/reforge
go test ./...
```

Before committing, run `gofmt -l .` (it should print nothing) and `go vet ./...`. The tests also run on Windows, Linux and macOS for every push and pull request.

The rules (what to update, which file to pick, when to ask) are in `internal/core` and are tested with a fake GitHub and a fake PC. Code that talks to the operating system is only in `internal/system`; the Windows parts have tests that use the real Windows APIs.

## Publishing a new version

On GitHub, create a release with a tag like `v1.0.1`. A GitHub Action runs the tests, builds `reforge` for Windows, macOS and Linux (x64 and ARM64), and attaches the files and `checksums.txt` to the release.
