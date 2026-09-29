# Plan 08 — `play` (experimental)

## Goal

Let an author or agent send a turn to a card from the terminal and read the
reply, so "edit → push → try" needs no browser. Generation costs credits, so
this command is opt-in per invocation and ships as experimental.

## Behaviour

```
hearthroom play [dir | --role <roleId>] -m "message" [--model id] [--allow-spend]
hearthroom play … --greeting 1        # start a new conversation from an alternate opening
hearthroom play … --history [--limit n]
hearthroom play … --stop
```

1. Resolve the role: the folder's pushed target from `.hearthroom/state.json`,
   or `--role`.
2. `POST /open/v1/conversation/start` opens or resumes the current conversation
   (`firstPageSize` fetches recent history in the same call). This does not
   generate and does not spend.
3. Without `--allow-spend`, `play -m` stops here and prints the cost notice and
   the exact flag to add. With it:
   - `POST /open/v1/conversation/ws-ticket`, then connect to
     `/open/v1/conversation/ws?protocolVersion=2`, send
     `{"type":"auth","ticket":…}`, wait for `ready`;
   - send one turn with `conversationId`, `message`, a fresh
     `clientOperationId` (UUID, persisted so a retry reuses it), `operationKind:
     "send"`, and `model` (from `--model`, else the first `runtimeEnabled`
     model in `GET /open/v1/models` that supports the card);
   - stream `answer` deltas to stdout, `thinking` to stderr only with
     `--show-thinking`, and finish on `done`; print the final
     `operationStatus` summary (credits settled, state).
4. `Ctrl-C` sends `POST /open/v1/conversation/stop` before exiting; the
   operation's final state is fetched and shown so the user knows what was
   charged.
5. `--json` emits one JSON object per frame (NDJSON) for agents.

## What the local provider can verify

A local provider has no upstream model by itself. With a small fake
OpenAI-compatible relay behind HTTPS (see docs/testing.md) the end-to-end
suite covers start, resume, history, ticket, handshake, the full event stream
of one turn, settlement and the `--allow-spend` gate. Generation quality is
not what is being tested; the wire path is.

Implementation note: the provider's WebSocket route accepts
`protocolVersion=2` today, so the CLI sends 2.

## Tests

- Unit: frame parser (SSE-formatted WebSocket frames), spend gate, model
  selection, reconnect bookkeeping (`resumeStreamId`, `lastEventId`).
- E2E (env-gated): start + ticket + auth frame + `ready` against the local
  provider; `-m` without `--allow-spend` never opens the socket.

## Acceptance

- `play -m "hi"` without `--allow-spend` exits 3 with the notice and makes no
  paid request.
- With `--allow-spend` on a real account, the reply streams and the settled
  status prints at the end.
