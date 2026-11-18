# Card tools

Declare tools in `card.json` when your card needs a validated decision object to drive its own UI. Each turn is one model call. The server checks the tool call against your schema but never runs it; your card acts on it and writes the outcome into the next turn's text.

```json
{
  "tools": [{
    "name": "report_turn",
    "description": "Choose a synthetic move for this turn.",
    "schema": {
      "type": "object",
      "properties": {"move": {"type": "string", "enum": ["a", "b", "c"]}},
      "required": ["move"],
      "additionalProperties": false
    }
  }]
}
```

Add the declaration to your existing manifest. Omission preserves the server declaration; `"tools": []` clears it. Push, pull and local validation preserve the field for owned and trial configurations. The server stores it in the private agent configuration and returns it only to authorized author reads, never public details or presentation assets.

Use a released SDK with the feature available, and detect it before passing options. An old JavaScript SDK may ignore extra arguments.

```js
if (!sdk.message.capabilities?.().tools) throw new Error('Card tools unavailable');
const capabilities = sdk.model.get().capabilities;
const canForceTool = capabilities?.tools === 'supported' && capabilities?.forcedTools === 'supported';
await sdk.message.send('Choose a move.', canForceTool ? {tool: 'report_turn'} : undefined);
sdk.on('message:done', message => {
  if (message.toolCall?.valid) drawDecision(message.toolCall.args);
  else if (message.toolCall) handleInvalidArguments(message.toolCall.args);
  else if (message.reason) handleTextFallback(message.content, message.reason);
});
```

- `sdk.message.send` resolves when the server accepts the turn, not when the model finishes. Wait for `message:done`.
- If the server refuses the send before the turn starts, the Promise rejects with an error code (for example `INSUFFICIENT`). Nothing was sent or charged; handle the error instead of waiting for `message:done`.
- `message:done` and `sdk.history.list({content:true})` carry `toolCall:{name,args,valid,error?}`. Valid `args` is an object; invalid `args` is the model's raw string plus a fixed `error` code.
- Treat `args` as untrusted: escape it before showing it, and never run it as code.
- The server does not run the call, send a tool result back, loop or retry.
- Prose, if the model writes any, follows the player's streaming setting; `args` arrive only when the turn finishes. A turn with only a tool call is still one AI message.
- Editing the message text clears its `toolCall`. Regenerating replaces it, or keeps the previous reply if regeneration fails. Forks keep it; rollback, deletion and reset follow the message. History requests with `content:false` omit `args`.

A turn selects one declared name. It cannot submit a schema override. Effective Agent or world mode with a tool is rejected before spending. Unknown names are rejected before spending. If this lane has not been verified to support forced tool calls, the turn runs as ordinary text on the same model and returns `toolCall:null` with `reason:"unsupported"`. Other null reasons: `"missing"` (no call), `"multiple_calls"`, `"interrupted"` (Stop or truncation; a partial call is never published as complete). The server never switches model or supplier silently.

From the CLI:

```sh
hearthroom play ./my-card -m "Choose a move." --tool report_turn --agent off --allow-spend --json
```

`--allow-spend` is required, as for any turn. Before sending, the CLI checks that the server supports card tools. A fixed-price model charges one turn's price; a usage-priced model charges the full request, including the tool arguments. Invalid JSON or schema is not retried and is still charged for the usage it consumed. The existing exemptions for server-side failures and for Stop still apply.

A context declared in `card.json` can run a tool the same way; see [Card contexts](card-contexts.md).

## Schema limits

The server is authoritative; the CLI uses the same validator. The accepted subset is JSON Schema 2020-12 with a root object, types, properties, required, additionalProperties, propertyNames, enum/const, array/string/numeric bounds, anyOf/oneOf/allOf and local `$defs`/`$ref`. Remote and cyclic references, duplicate JSON keys, unknown keywords and duplicate names are rejected.

- At most 16 declarations; names match `^[A-Za-z][A-Za-z0-9_]{0,63}$`.
- Reserved names: `submit_reply`, `write_draft`, `finish_preparation`, `record_format_notes`.
- Description: 2 KiB UTF-8; schema: 32 KiB each, depth 32; complete group: 128 KiB; arguments: 64 KiB.
- String lengths use Unicode code points. Referenced schema graphs have a bounded expansion budget.

Game-specific legality, authorization, costs and turn settlement remain the card's responsibility. Static schemas may express an operation enum; per-turn subsets and dynamically replaced schemas are not supported.

`hearthroom models` and `/open/v1/models` list each lane's capabilities: `tools`, `forcedTools`, `jsonSchema` and `imageInput`, each `supported`, `unsupported` or `unknown`. `unknown` means not verified yet; it does not mean supported. `jsonSchema` means the model's native strict response format, separate from tool-argument schemas. Capabilities are verified per lane and per thinking setting, and a verification stops applying when the lane changes. Whether a model can run in Agent mode is a separate setting.

Before sending, read `sdk.model.get().capabilities` for the player's current lane and thinking setting. Read it again when `model:change` fires; capability updates also trigger that event. Missing evidence and older hosts give `unknown` (an older SDK may omit the object). Only select a tool when both `tools` and `forcedTools` are `supported`. For `unknown` or `unsupported`, offer the card's ordinary text path without trying another paid model call. This choice does not make a text turn free; normal usage charges still apply.

MCP is not applicable to this server, which has no MCP transport. External AI clients use this CLI and the public skills.
