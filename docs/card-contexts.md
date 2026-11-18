# Card contexts

A context is a separate model conversation a card declares next to the chat itself, which scripts call `main`. Each context has its own history, private preset, Lorebooks and tools, and may pin a model family. Use one when a part of the card must reason only from what it is handed (a faction, a character, a referee) or must answer in its own format. Players see each context the card has used as a tab beside `main`; its output never enters `main` unless your script sends it there.

This page covers running contexts from the chat page and what the CLI does for them. Declaration files (`contexts/<id>.json`, the preset file, named Lorebooks) are described in the [card folder guide](card-folder.md#contexts-and-named-lorebooks).

Contexts need a provider that has turned them on. Until then, push refuses a card that declares them, and `sdk.capabilities().contexts` is `false`. Feature-detect it before every new call (`sdk.capabilities?.().contexts`, so an older SDK without the method does not throw); without it, each `sdk.context`, `sdk.batch` and `sdk.state.set` call, and each new `message.send` option, throws `NOT_SUPPORTED`. A new option is never sent as plain text on an older host. Only sandbox chat pages have `sdk.context`; classic pages do not. There is no tool-protocol server for this feature either: external agents use the CLI and the public skills.

## Check before you push

`hearthroom card push` calls the author-only preflight `POST /open/v1/role/context-preflight` (scope `role.write`) before it uploads anything. The server compiles every declared preset with the same compiler that will run it and returns, per context, the fixed-prefix size, the included materials and a provider-estimated credit range. Fixed material that does not fit `contextBudget` is an error; nothing is trimmed for you. The CLI shows the numbers as estimates and does not price anything itself. Workshop drafts use `POST /open/v1/workshop/context-preflight` through `hearthroom workshop update`.

A host or integration that needs the platform switch can read it: `GET /open/v1/role/author-asset/serve?...&includeFeatures=cardContexts` adds `features.cardContexts` (a boolean for the whole platform, not per card). A provider that predates it leaves the field out; treat that as off.

## Run a context

```js
if (!sdk.capabilities?.().contexts) return showPlainFlow();

const quote = await sdk.context.quote('settle', {turn: 7});
if (!(await askPlayer(quote))) return;          // your own dialog; the platform adds none

const result = await sdk.context.run('settle', {turn: 7}, {instance: 'north', label: 'North'});
if (result.status === 'complete' && result.toolCall?.valid) applyMove(result.toolCall.args);
else if (result.status === 'complete') showText(result.text);
```

`run` resolves when the call ends. `status` is `complete`, `interrupted`, `stopped`, `failed`, `moderated` or `discarded`, and each is a normal resolution; only a refusal before any spending rejects. The result carries `callId`, `text`, `toolCall` and `reason` (the same shapes as card tools), `charged` and `settled`. `settled: false` means the charge is not final yet.

| Option | Meaning |
| --- | --- |
| `instance` | Short key (`[A-Za-z0-9_-]`, 1–64). One declaration can have many instances, each with its own history and memory, up to 64 per declaration and 256 per conversation. Omitted is the default instance |
| `label` | Name shown on the instance chip (≤24 characters), set when the instance is created |
| `worldbooks` | Subset of the declared books for this instance, fixed at creation. A different list later is `CONFLICT` |
| `source` | Omit for the card; a DLC's `workshopId` from `sdk.workshop.list()` for a DLC |
| `tool` | One declared tool name. With a single declared tool it is chosen automatically |
| `messages` | Required for `assembly: "client"`, refused for platform assembly |
| `batch` | A `batchId` from `sdk.batch.open` |
| `idempotencyKey` | Same key and intent returns the same call and charges once. The SDK makes one if you omit it |
| `onDelta`, `signal` | Streaming text (declared `stream: true` only) and `AbortSignal` stop |

One call per instance runs at a time (`BUSY`); `main` and other instances run in parallel, up to a platform limit (default 16). `sdk.context.stop(callId)` stops one call, `list(id, opts)` reads that context's recorded entries, `instances()` lists instances that already have records, and `truncate(id, afterSeq, opts)` stops the instance and keeps history up to a sequence number. No call lists declarations, presets, Lorebooks or tools. Unknown ids, disabled sources and denied calls all return the same `CONTEXT_UNAVAILABLE`.

## History and who assembles it

| `history` | Behavior |
| --- | --- |
| `append` | History grows; the platform compacts it when needed (charged at the standard rate) |
| `none` | Every call stands alone; records are kept but not sent back |
| `window` | Recent turns kept; older turns dropped in one large step, never summarized |

`assembly: "platform"` is the default: the platform assembles history and keeps the prefix cache stable for that context. `assembly: "client"` is for CLI-pushed cards that assemble their own prompt: every `run` carries `messages` (1–1,000 entries of role `user` or `assistant`, at most 1 MiB, the last a non-empty `user`). The platform still adds the preset, Lorebooks, tools and its safety text on the server (and the player's persona when they have one), but it does not compact; a request that does not fit is refused before spending. Cache hits then depend on your script only appending.

What the player's own settings add to `main` (response preferences, standing instructions, notebook, card values, custom system prompt) never enters a context; pass what it needs as `vars`.

`include` lets a context read visible dialogue text from `main` (`{"from":"main","turns":4}`) or from another context (`{"from":"id","instance":"key","entries":4}`). It never carries a preset, a Lorebook or a summary.

`"main": {"assembly": "client"}` in `card.json` applies the same idea to the chat itself. Register `sdk.main.onCompose(text => messages)`; the host asks it on every send. Regenerate reuses the frozen messages, Continue is not offered, and Agent and world modes are refused before spending. The platform guarantees prefix-cache stability only for platform assembly.

## Tools and models

A context may list only the card tools declared by the same source: the card's own, or a DLC's own tools. See [Card tools](card-tools.md) for declarations and schemas. Before choosing a tool, read `sdk.model.get().capabilities`; `tools` and `forcedTools` are `supported`, `unsupported` or `unknown`. `unknown` means the platform has no measurement for the selected line and thinking setting. Treat it like `unsupported`: offer ordinary text, which still bills at the normal rate. The server never swaps model or supplier on its own.

A declaration's `model.family` restricts the context to one public model family. Without it the player's current model is used. A card can let the player pick with `sdk.context.models(id)` and `sdk.context.setModel(id, modelId)`; a model that cannot do the context's tool is `MODEL_UNSUPPORTED`.

## Cost, quotes and batches

`sdk.context.quote(id, vars, opts)` and `quoteMany([{id, vars, ...}])` (1–16 items, same order back, each `{ok, quote}` or `{ok:false, code}`) return whole credits without creating an instance or calling a model:

| Field | Meaning |
| --- | --- |
| `expectedCharge` | Typical charge, or `null` when there is not enough data. `null` is unknown, not free |
| `maxCharge` | Admission estimate, `requestMaxCharge` + `compactionMaxCharge` |
| `generatedAt`, `expiresAt` | A quote lasts at least 60 seconds and is not a locked price |

The final charge follows actual usage, as with any turn. `run` re-estimates before spending and returns `CHARGE_LIMIT` if the estimate exceeds the declaration's `maxChargePerCall`. The platform does not force a confirmation. Draw your own dialog from the quote, ask once for several calls, or call without asking; the page, not the script, reads the click gesture.

For several calls under one approval, open a batch:

```js
const batch = await sdk.batch.open({maxCharge: 120});
await Promise.all([
  sdk.context.run('diplomat', {a: 1}, {batch: batch.batchId}),
  sdk.context.run('advisor', {a: 1}, {batch: batch.batchId}),
]);
const done = await sdk.batch.close(batch.batchId);  // {status: 'closing'|'closed', charged, settled}
```

Calls draw only from the batch reserve and never top up from the player's wallet; a shortfall is `INSUFFICIENT` before spending. `closing` means a call is still settling and `charged` is the running total; close again later for the final value. A call on a closed or expired batch is `BATCH_CLOSED`. `message.send(text, {batch})` also joins a batch.

Two limits apply per account. Quotes and `batch.open` share 60 requests per minute (`RATE_LIMITED`, with a retry delay; `quoteMany` counts per item). Separately the server watches call frequency: an unusual, sustained rate is recorded and then paused (`PAUSED`, with a retry delay). Normal play, including background calls from your script, is not capped by credit amount.

## Getting a result into `main`

Nothing is written back automatically. Choose what your script does:

- `sdk.message.send(text, {as: 'card', from: {context: 'north'}})` sends a player-role message that the page labels as sent by the card, with "@" and the context's name. The model sees an ordinary player message.
- `sdk.state.set({money: 73})` writes card values (existing paths only, 16 KiB at most, `BUSY` while `main` is generating). It calls no model.
- Pass the result as `vars` to another context.

Showing a context's output as an AI message in `main` is not available.

## Hidden contexts and what stays private

`visible: false` hides the tab's contents and its export from the player. Call counts and credits still show. This is not secrecy: your script reads the recorded outputs and so can anyone with browser developer tools. Nothing on the page can list a declaration, preset, Lorebook name or entry, but any output a player sees can be copied, and prompt injection can try to draw private material out of any model. Keep high-value private material out of any context with `playerLore: "allow"`.

## Workshop

An author's DLC may declare contexts bound to its own Lorebook entries and its own tools; a player extension may not. A DLC context runs only while that DLC is enabled in the conversation. Pass `source: workshopId` for it.

## Errors

| Code | Cause |
| --- | --- |
| `NOT_SUPPORTED` | The host or platform has no contexts |
| `INVALID_ARGS` | Bad arguments, or `messages` mismatched with the assembly |
| `CONTEXT_UNAVAILABLE` | Unknown, disabled or denied context |
| `CONFLICT` | Same idempotency key with another intent, or changed instance books or label |
| `BUSY` | Instance already running, concurrency limit, or `main` generating for `state.set` |
| `LIMIT_REACHED` | Instance count limit |
| `INSUFFICIENT` | Credits or batch budget too low |
| `CHARGE_LIMIT` | Estimate above `maxChargePerCall` |
| `PAUSED`, `RATE_LIMITED` | Frequency pause or quote rate limit; wait the retry delay |
| `MODEL_UNSUPPORTED` | Selected model cannot do the context's tool |
| `BATCH_CLOSED` | Batch closed, expired or unknown |
| `NETWORK` | Anything else |
