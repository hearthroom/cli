---
slug: "hearthroom"
command: "hearthroom"
short: "Write, push, validate and import character cards for Hearthroom"
---

# hearthroom

hearthroom is the command-line client for Hearthroom.

Cards live in folders you can version and hand to an AI agent. Push a folder to
a private trial card on the connected provider, validate it, play it, pull it
back, or import cards from SillyTavern and MMD. Every command accepts --json.

Agents: start at https://sukisuki.ai/llms.txt for the card model, what needs
sign-in or spends credits, and the Markdown sources of the guide and API reference.

## Commands

- [`hearthroom auth`](/manual/auth/) — Sign in to the connected card provider
- [`hearthroom author`](/manual/author/) — Show a community author
- [`hearthroom card`](/manual/card/) — Work with card folders: init, push, validate, pull, import, list
- [`hearthroom completion`](/manual/completion/) — Shell completion: install, uninstall, or print the script
- [`hearthroom media`](/manual/media/) — Upload and list files in your media library
- [`hearthroom models`](/manual/models/) — List models the provider offers
- [`hearthroom play`](/manual/play/) — Talk to a card from the terminal (experimental)
- [`hearthroom search`](/manual/search/) — Browse or search the community board (no sign-in needed)
- [`hearthroom tags`](/manual/tags/) — List community tags
- [`hearthroom upgrade`](/manual/upgrade/) — Update hearthroom to the latest release
- [`hearthroom version`](/manual/version/) — Print the version
- [`hearthroom wallet`](/manual/wallet/) — Show your credit balance and plans
- [`hearthroom whoami`](/manual/whoami/) — Show the signed-in account on the provider and the community site

## Options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://hearthroom.club)
```

