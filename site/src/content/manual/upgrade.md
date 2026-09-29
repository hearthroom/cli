---
slug: "upgrade"
command: "hearthroom upgrade"
short: "Update hearthroom to the latest release"
parent: "hearthroom"
---

# hearthroom upgrade

Downloads the latest release for this platform from GitHub Releases, verifies
it against the release's checksums.txt, and replaces the running binary.

If hearthroom was installed with a package manager (Homebrew, Scoop), use that
manager's upgrade command instead; this command will tell you when it notices.

## Usage

```
hearthroom upgrade [flags]
```

## Options

```
      --check   only report whether a newer release exists
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
