---
slug: "auth/login"
command: "hearthroom auth login"
short: "Sign in with a one-time code (works over SSH)"
parent: "auth"
---

# hearthroom auth login

Signs in to the provider behind --api. The CLI prints a one-time code and a
web address. Open the address on any device (this computer, a laptop, a
phone), sign in, and enter the code; the CLI picks up the sign-in as soon as
you approve it. On a desktop the page opens in your browser; type the code
there. Over SSH, or with --no-browser, nothing is opened.

If the provider does not offer sign-in with a code, the CLI says so and signs
in through a browser on this machine instead (OAuth with PKCE, receiving the
result on a loopback port).

Agents that only see a command's output after it exits can split the wait:
--no-wait prints the code and address (one JSON object with --json) and exits;
after the person has approved, --resume finishes the sign-in.

Tokens are stored under the config directory with owner-only permissions and
refreshed automatically. For scripts and CI, set HEARTHROOM_TOKEN instead of
signing in.

## Usage

```
hearthroom auth login [flags]
```

## Options

```
      --no-browser         do not open a browser; just print the code and address
      --no-wait            print the code and address, then exit; finish later with --resume
      --resume             finish the sign-in started with --no-wait
      --timeout duration   how long to wait for the sign-in to be approved (default 5m0s)
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom auth`](/manual/auth/) — Sign in to the connected card provider
