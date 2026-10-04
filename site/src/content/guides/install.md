---
title: Installing
---

# Installing

`hearthroom` is a single binary for Linux, macOS and Windows on amd64 and arm64. Nothing to compile, no runtime to install.

## macOS

```sh
brew install hearthroom/tap/hearthroom
```

The cask installs the binary and shell completions from the GitHub release and clears the macOS quarantine flag, so Gatekeeper does not block the first run. `brew upgrade hearthroom` updates it. The shell script below works on macOS too.

## Linux (and macOS without Homebrew)

```sh
curl -fsSL https://raw.githubusercontent.com/hearthroom/cli/main/install.sh | sh
```

The script picks the build for your platform from the latest GitHub release, checks it against the release's `checksums.txt`, installs it to `~/.local/bin` (or `/usr/local/bin` when run as root; override with `HEARTHROOM_INSTALL_DIR`), and sets up tab completion for bash, zsh or fish. Pin a version with `HEARTHROOM_VERSION=v0.1.5`.

Files fetched with `curl` carry no macOS quarantine flag, so Gatekeeper does not block the first run.

## Windows

With [Scoop](https://scoop.sh):

```powershell
scoop bucket add hearthroom https://github.com/hearthroom/scoop-bucket
scoop install hearthroom
```

Or with the PowerShell script:

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

## Updating

`hearthroom` checks for a newer release at most once a day and prints one line when there is one. Homebrew and Scoop installs update through their package manager; for the script and manual installs, run:

```sh
hearthroom upgrade
```

Set `HEARTHROOM_NO_UPDATE_NOTIFIER=1` to silence the check.

## Signing in

```sh
hearthroom auth login
```

Prints a one-time code and an address. Open the address on any device, sign in, and enter the code; the command finishes as soon as you approve. On a desktop the page opens in your browser for you to type the code, and over SSH or on a machine without a browser you open it somewhere else. If a provider does not offer sign-in with a code, the CLI says so and opens the provider's sign-in page in a browser on this machine instead, receiving the result on a local port (41777–41781). Tokens are stored under your user config directory with owner-only permissions and refreshed automatically. For scripts and CI, set `HEARTHROOM_TOKEN` instead.

## Shell completion

The installer sets it up. To do it later or on another shell:

```sh
hearthroom completion install            # detects bash, zsh or fish
hearthroom completion install --shell fish
hearthroom completion uninstall
```
