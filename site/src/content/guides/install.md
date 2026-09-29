---
title: Installing
---

# Installing

`hearthroom` is a single binary for Linux, macOS and Windows on amd64 and arm64. Nothing to compile, no runtime to install.

## macOS and Linux

```sh
curl -fsSL https://raw.githubusercontent.com/hearthroom/cli/main/install.sh | sh
```

The script picks the build for your platform from the latest GitHub release, checks it against the release's `checksums.txt`, installs it to `~/.local/bin` (or `/usr/local/bin` when run as root; override with `HEARTHROOM_INSTALL_DIR`), and sets up tab completion for bash, zsh or fish. Pin a version with `HEARTHROOM_VERSION=v0.1.5`.

Files fetched with `curl` carry no macOS quarantine flag, so Gatekeeper does not block the first run.

## Windows

```powershell
irm https://raw.githubusercontent.com/hearthroom/cli/main/install.ps1 | iex
```

Installs to `%LOCALAPPDATA%\Programs\hearthroom`, adds it to your user PATH and clears the download mark so SmartScreen does not prompt. For PowerShell completion, add this line to your profile:

```powershell
hearthroom completion powershell | Out-String | Invoke-Expression
```

## Other ways

- **From source** (Go 1.26+): `go install github.com/hearthroom/cli/cmd/hearthroom@latest`
- **Manual download**: unpack the archive for your platform from the [latest release](https://github.com/hearthroom/cli/releases/latest) and put the binary on your PATH. Every asset has a build provenance attestation: `gh attestation verify <file> --owner hearthroom`.
- Homebrew and Scoop packages are planned.

## Updating

`hearthroom` checks for a newer release at most once a day and prints one line when there is one. Update with:

```sh
hearthroom upgrade
```

Set `HEARTHROOM_NO_UPDATE_NOTIFIER=1` to silence the check.

## Signing in

```sh
hearthroom auth login
```

Opens the provider's sign-in page in your browser and receives the result on a local port (41777–41781). Tokens are stored under your user config directory with owner-only permissions and refreshed automatically. For scripts and CI, set `HEARTHROOM_TOKEN` instead.

## Shell completion

The installer sets it up. To do it later or on another shell:

```sh
hearthroom completion install            # detects bash, zsh or fish
hearthroom completion install --shell fish
hearthroom completion uninstall
```
