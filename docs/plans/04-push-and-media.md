# Plan 04 — `card push` and media

## Goal

Sync a card folder to the provider in one command, uploading local assets and
rewriting references, so the author can play the result immediately.

## Targets

- **Trial card (default).** `PUT /open/v1/trial-cards/{clientKey}` with the
  four sections. The key is stored in `.hearthroom/state.json` (generated from
  the folder name plus a random suffix on first push, letters/digits/`_`/`-`,
  ≤ 64 chars). The server returns per-section content hashes; the CLI stores
  them and only re-sends sections whose local hash differs. `--evict` passes
  the server flag that frees a slot when all trial slots are used.
- **Owned card.** `card push --to <roleId>` writes to a private card the caller
  owns: `POST /role/{roleId}/document` (fields), `PATCH /role/{roleId}/welcome`
  (opening, alternates, prologue), `PUT /role/{roleId}/author-asset` (rules,
  with the last-read `version` for optimistic concurrency), and
  `POST /worldbook/{id}/document` for the bound Lorebook (create and bind first
  when none is bound). `--create` creates a new private card with
  `POST /open/v1/role` and records its id.

Both paths print what changed and the URL to play the card on the community
site (`<site>/play/<roleId>`; the site resolves availability).

## Media

1. Collect references: `card.json.media.*` and any `assets/…` relative path
   found in `welcome.md`, `openings/*.md`, `definition.md`, `rules.json` and
   Lorebook entry content (regex on `assets/[^\s"')]+`).
2. Read `GET /open/v1/image/list` once to learn `capabilities.relativePaths`
   and `libraryPrefix`. When `relativePaths` is true, upload with
   `POST /open/v1/image/upload` and `relativePath=<card-key>/<path under assets>`;
   the served URL is `<libraryPrefix>/<relativePath>` and same-path uploads
   replace content. When false, upload without a path and use the returned
   `imageUrl`.
3. Skip unchanged files: `.hearthroom/state.json` records the SHA-256 and served
   URL of every uploaded asset.
4. Rewrite references to served URLs **in the payload only**; local files keep
   relative paths so the folder stays portable.
5. Accepted types and the size limit come from the endpoint contract
   (images, mp4/webm, mp3/wav/ogg, woff/woff2/ttf/otf; 100 MB). Rejections are
   reported per file; the push continues for the rest and exits non-zero.

`hearthroom media upload <file> [--path rel]` and `media ls [--q text]` expose
the library directly.

## Safety

- `push` never deletes remote content. Removing a Lorebook entry locally
  produces a `delete` op only for entries the CLI itself created or pulled
  (tracked by id in state).
- `--dry-run` prints the sections and files that would be sent.

## Tests

- Unit: reference collection and rewriting, hash/skip logic, section diffing.
- E2E (env-gated): init → push (trial) → assert server `sections` hashes match →
  edit `welcome.md` → push → only `welcome` in `changed`.

## Acceptance

- Two consecutive pushes without edits send no sections.
- An image referenced from `welcome.md` is uploaded once and served from the
  library prefix.
