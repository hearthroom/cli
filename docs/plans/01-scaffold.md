# Plan 01 — Scaffold

## Goal

A buildable `hearthroom` binary with the command tree, configuration, output
conventions and release tooling in place, so every later plan only adds verbs.

## Layout

```
cmd/hearthroom/main.go        entry point
internal/cli/                 cobra commands (one file per verb group)
internal/config/              config dir, config.json, credentials.json
internal/api/                 HTTP client for /open/v1 and /v1 (typed helpers)
internal/auth/                OAuth PKCE, client registration, token store
internal/card/                folder format, hashing, diff
internal/importer/            SillyTavern PNG/JSON/CHARX, MMD three-file set
internal/media/               upload, relative-path rewriting
internal/output/              human vs --json rendering
internal/e2e/                 end-to-end tests (env-gated)
docs/plans/                   these plans
docs/card-folder.md           the folder format contract
```

## Conventions

- Module `github.com/hearthroom/cli`, Go 1.26, `cobra` for commands.
- Global flags: `--api <url>` (provider API base), `--site <url>` (community
  site), `--json` (machine output), `--profile <name>` (config profile).
- Environment: `HEARTHROOM_API`, `HEARTHROOM_SITE`, `HEARTHROOM_TOKEN`
  (bearer for non-interactive use), `HEARTHROOM_CONFIG_DIR`.
- Config lives in `os.UserConfigDir()/hearthroom/`; `credentials.json` is
  written with mode 0600.
- Every request sends `User-Agent: hearthroom-cli/<version> (<os>/<arch>)`.
- Errors: non-zero exit; with `--json`, a single `{"error": ..., "detail": ...}`
  object on stdout. Without `--json`, a one-line message on stderr.
- `hearthroom version` prints version, commit and build date (set by GoReleaser).

## Release tooling (set up now, exercised in plan 09)

- `.goreleaser.yaml`: linux/darwin/windows × amd64/arm64, `-trimpath`, no UPX,
  archives `tar.gz` (zip on Windows), `checksums.txt`.
- `.github/workflows/ci.yml`: `go test ./...` on ubuntu, macos, windows; `go vet`.
- `.github/workflows/release.yml`: on tag `v*`, GoReleaser + `actions/attest-build-provenance`.
- `install.sh` (POSIX) and `install.ps1`: detect OS/arch, download the latest
  release asset, verify against `checksums.txt`, place the binary on PATH.

## Acceptance

- `go build ./...` and `go test ./...` pass locally.
- `goreleaser release --snapshot --clean` produces all six binaries.
- `hearthroom version` and `hearthroom --help` work from a snapshot binary.
