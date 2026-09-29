---
slug: "auth/login"
command: "hearthroom auth login"
short: "Sign in with your browser (OAuth with PKCE)"
parent: "auth"
---

# hearthroom auth login

Signs in to the provider behind --api. The CLI registers itself as an OAuth
client once, opens the provider's sign-in page in your browser, and receives
the result on a loopback port. Tokens are stored under the config directory
with owner-only permissions and refreshed automatically.

For scripts and CI, set HEARTHROOM_TOKEN instead of signing in.

## Usage

```
hearthroom auth login [flags]
```

## Options

```
      --no-browser         print the sign-in URL instead of opening a browser
      --timeout duration   how long to wait for the browser (default 5m0s)
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://hearthroom.club)
```

## See also

- [`hearthroom auth`](/manual/auth/) — Sign in to the connected card provider
