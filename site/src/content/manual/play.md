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

Turns carry the card's language from card.json unless --language says otherwise;
without one the provider replies in English. --new-session archives the current
conversation with the card and starts a fresh one, so two folders can test the
same card without sharing a thread.

With --json, every server event is printed as one JSON object per line.

## Usage

```
hearthroom play [dir] [flags]
```

## Options

```
      --agent string              agent mode for this turn: on or off (default: saved preference)
      --allow-spend               confirm that this turn may spend credits
      --greeting int              opening for a new conversation (0 = main, 1.. = alternates); an existing conversation is resumed as is, so pair it with --new-session
      --history                   print recent messages instead of sending
      --language string           reply language, e.g. zh-Hant (default: the folder's card.json language; the provider assumes en when none is sent)
      --limit int                 messages to show with --history (default 20)
  -m, --message string            message to send (spends credits; requires --allow-spend)
      --model hearthroom models   model value from hearthroom models (default: provider default)
      --new-session               archive the current conversation with this card and start a fresh one
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
