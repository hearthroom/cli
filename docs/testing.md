# Testing

## Unit tests

```sh
go test ./...
```

Fixtures are built in the tests themselves (PNG chunks, CHARX archives, MMD
files, a fake OAuth provider, a fake WebSocket stream), so the suite runs
offline on Linux, macOS and Windows.

## End-to-end tests

`internal/e2e` runs the CLI's own code paths against a real provider. It is
skipped unless two variables are set:

| Variable | Meaning |
|---|---|
| `HEARTHROOM_E2E_API` | Provider API base, e.g. `http://localhost:8890` |
| `HEARTHROOM_E2E_SESSION` | Value of the provider's signed-in session cookie (`hh_session`) for a test account |
| `HEARTHROOM_E2E_SPEND=1` | Also run the paid turn in the play test (charges the test account) |

The session cookie is used only to approve the CLI's OAuth request headlessly,
the way a browser would; login, push, pull, validate, media and play then go
through the CLI packages. Use a disposable account on a local or staging
provider. Do not point the suite at a production account.

```sh
HEARTHROOM_E2E_API=http://localhost:8890 \
HEARTHROOM_E2E_SESSION=… \
go test ./internal/e2e/ -v -count=1
```

What the suite covers:

- login → `/me`, client registration reuse, logout and revocation;
- trial push with two assets (uploaded once, references rewritten), server
  validation, a no-op second push, a welcome-only third push, pull with
  media download and a file-level comparison;
- owned card create, Lorebook entry update and delete, display rule update,
  cleanup;
- conversation start and resume, history, the spend gate, and (with
  `HEARTHROOM_E2E_SPEND=1`) one streamed turn whose events arrive in order
  and whose reply lands in history.

### Local provider notes

A provider's conversation surface is usually enabled only when a model relay
is configured, and relay URLs must be HTTPS. For local runs the CLI was
verified against a provider whose relay pointed at a small fake
OpenAI-compatible server over TLS with a self-signed certificate trusted by the
provider process. The fake relay streams a fixed reply, so the paid turn
exercises the full path (ticket, handshake, frames, settlement) without an
upstream model.

The WebSocket route accepts `protocolVersion=2`; the CLI sends that.
