# Prompt presets

`hearthroom preset check` checks the file structure, Hearthroom's `suki_` fields,
and platform slots locally. It accepts SillyTavern JSON, version 1 prompt packs,
and `hearthroom.st-preset.v1` documents. When a SillyTavern file has several
prompt order profiles, pass `--profile-id` to choose one. Harbor evaluates full
compatibility at save time.

```sh
hearthroom preset check preset.json --json
hearthroom auth login --presets
hearthroom preset push preset.json --create --title "My preset"
hearthroom preset list
```

Preset access requires your consent on the first-party Hearthroom provider.
Existing sign-ins can get this access by signing in with `--presets`. For an
SSH workflow, start with `auth login --presets --no-wait` and finish with
`auth login --resume`. The CLI manages presets owned by your signed-in account.

A SillyTavern file goes through the same import preview and confirmation as the
website. This is the format to use for a new preset with `suki_` fields or
platform response slots. If the provider flags compatibility issues that need
choices, review its import preview in the web editor. Creating from a
Hearthroom document uses the native editor's rules.

## Export and edit

```sh
hearthroom preset pull "$PRESET_ID" --out tavern.json
hearthroom preset pull "$PRESET_ID" --format hearthroom --out edit.json --json
hearthroom preset check edit.json
hearthroom preset push edit.json --id "$PRESET_ID" --expected-version "$EXPORTED_VERSION"
```

The default export is the same SillyTavern JSON the website downloads.
`--format hearthroom` keeps the saved selection and reports the version from
the same read. Use that version as `EXPORTED_VERSION` after editing the file.
A conflict means the preset changed since your export. Pull to a new file and
reconcile the edits before saving again. Both formats create a new output file
with private permissions.

Updating an already shared preset changes the content others can use or see
according to its current sharing settings. The preset commands manage your
own files and saves. Use the web editor to manage sharing or apply a preset.

If the CLI cannot confirm a save, rerun the same command with the operation ID
it printed. Keep the same account, file and options so Harbor can match the
original request.
