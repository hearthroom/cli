---
slug: "review/author-note"
command: "hearthroom review author-note"
short: "Read or replace your private note on the submission's author"
parent: "review"
---

# hearthroom review author-note

Reads your note on the author of a submission. With --set or --file it
replaces the note (up to 2,000 characters); an empty note deletes it.

There is one note per author, shared by all of that author's submissions, and
author also returns it. Authors never see it. Use it to keep what you learned
about an author across reviews, such as a reused setting template or a pattern
in earlier rejections. Read the note first and include what should stay, since
writing replaces it.

--file reads plain text from a file, or from stdin with -.

## Usage

```
hearthroom review author-note <submissionId> [--set TEXT | --file PATH|-] [flags]
```

## Options

```
      --file string   replace the note with the text of this file ('-' for stdin)
      --set string    replace the note with this text (empty deletes it)
```

## Global options

```
      --api string          provider API base (default from config or https://api.harperharbor.com)
      --config-dir string   config directory (default $HEARTHROOM_CONFIG_DIR or the user config dir)
      --json                machine-readable JSON output
      --site string         community site (default from config or https://sukisuki.ai)
```

## See also

- [`hearthroom review`](/manual/review/) — Staff tools for card review (the server decides who may use them)
