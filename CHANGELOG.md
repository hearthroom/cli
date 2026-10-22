# Changelog

## 0.10.2 (2026-10-22)

- Disabled Lorebook entries are preserved on push, including their enabled state, matching options and groups.
- Deleting locally removed Lorebook entries from an owned card requires --prune; push lists planned deletions when the flag is absent.

## 0.10.1 (2026-10-21)

- Large Lorebooks follow the provider's operation limit and save progress after each batch; rerunning an interrupted push reuses its card and Lorebook.
- Long entries without author group settings are split without losing whitespace or punctuation; the CLI reports each grouped entry before writing.
- `card push --create` requests a private card. Private and unlisted access are explained separately in the manual.
- Trial pushes check the provider's entry limit before uploading; larger cards can use `--create`.
- Unknown batch outcomes are reconciled with the provider before new entries are sent again.
