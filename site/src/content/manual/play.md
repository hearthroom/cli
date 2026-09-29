---
slug: "play"
command: "hearthroom play"
short: "Talk to a card from the terminal (experimental)"
parent: "hearthroom"
---

# hearthroom play

Opens or resumes the current conversation with the card in a folder (its
pushed trial or owned card) or with --role <id>, and sends one message.

Starting a conversation and reading history are free. Sending a message
generates a reply and spends credits on the provider, so it runs only with
--allow-spend. Press Ctrl-C to stop a reply; what was generated is still
charged and shown.

With --json, every server event is printed as one JSON object per line.

## Usage

```
hearthroom play [dir] [flags]
```

## Options

```
      --agent string              agent mode for this turn: on or off (default: saved preference)
      --allow-spend               confirm that this turn may spend credits
      --greeting int              opening to use when a new conversation is created (0 = main, 1.. = alternates)
      --history                   print recent messages instead of sending
      --limit int                 messages to show with --history (default 20)
  -m, --message string            message to send (spends credits; requires --allow-spend)
      --model hearthroom models   model value from hearthroom models (default: provider default)
      --role string               card id instead of a folder
      --show-thinking             print reasoning deltas to stderr
      --stop                      stop the reply currently being generated
      --thinking string           thinking depth override for models that support it
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
