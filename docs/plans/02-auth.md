# Plan 02 — Authentication

## Goal

`hearthroom auth login` obtains a provider access token, stores it, refreshes
it silently, and lets non-interactive callers supply a token instead. By
default it signs in with a one-time code (OAuth 2.0 device authorization
grant, RFC 8628), the way `gh auth login` does, so it works on SSH sessions
and cloud machines. Providers that do not offer the code get the OAuth 2.1
authorization-code flow with PKCE on a loopback port.

## Flow

1. Discover endpoints from `<api>/.well-known/oauth-authorization-server`.
   `device_authorization_endpoint` is optional; its absence means the
   provider has no sign-in with a code.
2. Use the first-party client the provider's operator issued for this CLI
   when one is known for the API base (`auth.KnownClients`); otherwise
   register a public client once with `POST /oauth/register`
   (`token_endpoint_auth_method: none`) and cache `client_id` in config.
   This matters because the provider namespaces data by the OAuth client's
   tenant: a dynamically registered client lands in the open tenant and
   cannot see cards the author made on the community site. The first-party
   client belongs to the community application, so the CLI and the site see
   the same inventory.
   Registration requests the scopes `profile.read email.read role.read role.write chat.play`.
   A provider may allow sign-in with a code only for clients somebody is
   accountable for (the first-party client), never for anonymous dynamic
   registrations.
3. Sign-in with a code (default):
   - `POST <device_authorization_endpoint>` with `client_id`, `scope` and
     `resource=<api>/open/v1` returns `device_code`, `user_code`
     (`BCDF-GHJK`), `verification_uri`, `verification_uri_complete`,
     `expires_in` and `interval`.
   - The CLI prints, on stderr, the code, then `verification_uri`, then that it
     opened `verification_uri_complete` in the browser. It does not open a
     browser with `--no-browser`, in an SSH session (`SSH_CONNECTION`,
     `SSH_CLIENT` or `SSH_TTY` set), or when no opener exists.
   - It polls `/oauth/token` with
     `grant_type=urn:ietf:params:oauth:grant-type:device_code`, `device_code`
     and `client_id`, waiting `interval` seconds between requests:
     `authorization_pending` keeps polling, `slow_down` adds 5 s to the
     interval, `access_denied` stops with "Sign-in was denied in the
     browser.", `expired_token` stops with "The code expired. Run
     `hearthroom auth login` again." The whole wait is bounded by `expires_in`
     and `--timeout` (default 5 minutes), whichever ends first.
4. Fallback: when discovery has no device endpoint, or the endpoint answers
   `unauthorized_client`, `unsupported_grant_type` or 404, the CLI prints one
   line saying so and runs the loopback flow unchanged. Any other error from
   the device endpoint (`invalid_scope`, `invalid_client`, a 429
   `rate_limited` with `retry_after`, …) is reported, not papered over.
5. Loopback flow (fallback). The provider matches loopback redirect URIs
   **exactly, including the port**, so the client is registered with a fixed
   list (`http://127.0.0.1:<port>/callback` for ports 41777–41781) and login
   binds the first free one; if none is free it reports which ports are
   needed. The browser opens `/oauth/authorize` with `code_challenge` (S256),
   `state`, `resource=<api>/open/v1` and the scopes (`--no-browser` prints the
   URL). The loopback handler checks `state`, exchanges the code at
   `/oauth/token` with `code_verifier`, and shows a plain success page. This
   needs a browser on the same machine.
6. Tokens are stored per API base in `credentials.json` (0600) with expiry and
   the client id, the same way for both flows. Requests refresh with the
   `refresh_token` when the access token has less than two minutes left; a
   failed refresh asks the user to log in again.
7. `auth logout` revokes the token at `/oauth/revoke` (best effort) and deletes
   the stored entry. `auth status` prints the account from `GET /open/v1/me`.

## Two-step sign-in for agents

Agents usually see a command's output only after it exits, so a blocking
login that prints a code and waits is useless to them. The wait splits in two:

- `auth login --no-wait` requests a code, prints the code and the address
  (with `--json`, one object `{"user_code", "verification_uri",
  "verification_uri_complete", "expires_in", "interval"}` on stdout), stores
  the request in `pending-login.json` (0600: api base, client id,
  `device_code`, interval, `expires_at`, plus the code and address for the
  status line) and exits 0 without opening a browser. It fails when the
  provider has no sign-in with a code; loopback cannot outlive the process.
- `auth login --resume` polls that request at once, then at the interval,
  saves the credential, deletes the pending file and prints the same result
  as a normal login. A used, denied or expired request is deleted (polling a
  used device code again counts as a replay and revokes the new tokens); a
  `--timeout` leaves it in place so the next `--resume` keeps waiting.
- `--resume` with nothing pending, with a request for another API base, or
  with an expired request fails and says to run `auth login --no-wait` again.
- `--no-wait` and `--resume` are mutually exclusive; a new `--no-wait`
  replaces any pending request.

## Non-interactive use

`HEARTHROOM_TOKEN` takes precedence over stored credentials. It may hold a
provider access token or a personal API key (`hh_live_…`) if the provider
issues them; the CLI does not distinguish. No refresh is attempted for it.

## Community site

The community `/v1` endpoints accept the same bearer token as the provider,
so a single login serves both hosts. The CLI sends it to the site only for
member endpoints; public browsing stays anonymous.

## Tests

- Unit: PKCE verifier/challenge, state check, token store round-trip and
  permission bits, refresh decision.
- Device flow against an httptest provider with an injected clock: discovery
  with and without the endpoint; pending → slow_down → success (the interval
  grows, the credential is stored); access_denied; expired_token; the wait
  bounded by `expires_in` and `--timeout`; fallback to loopback on a missing
  endpoint, `unauthorized_client`, `unsupported_grant_type` and 404; no
  browser in an SSH session; `--no-wait` writes the 0600 pending file and
  prints the JSON object; `--resume` completes and deletes it; `--resume`
  errors (nothing pending, another API base, expired).
- E2E (env-gated): headless loopback login (`LoopbackOnly`) against a local
  provider by driving the consent endpoints with a session cookie obtained
  through a seeded email code, then `GET /open/v1/me` through the CLI.

## Acceptance

- `hearthroom auth login` on a desktop completes in the browser and
  `hearthroom auth status` shows the account.
- Over SSH, `hearthroom auth login` prints a code and an address, opens
  nothing, and completes once the code is approved on another device.
- `hearthroom auth login --no-wait --json` followed by
  `hearthroom auth login --resume` signs an agent in.
- `HEARTHROOM_TOKEN=… hearthroom auth status` works with no config dir.
