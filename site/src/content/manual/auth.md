---
slug: "auth"
command: "hearthroom auth"
short: "Sign in to the connected card provider"
parent: "hearthroom"
---

# hearthroom auth

Sign in to the connected card provider

## Commands

- [`hearthroom auth login`](/manual/auth/login/) — Sign in with your browser (OAuth with PKCE)
- [`hearthroom auth logout`](/manual/auth/logout/) — Forget the stored sign-in for this provider
- [`hearthroom auth status`](/manual/auth/status/) — Show who you are signed in as

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://hearthroom.club)
```

## See also

- [`hearthroom`](/manual/hearthroom/) — Write, push, validate and import character cards for Hearthroom
