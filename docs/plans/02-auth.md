# Plan 02 — Authentication

## Goal

`hearthroom auth login` obtains a provider access token through the standard
OAuth 2.1 authorization-code flow with PKCE, stores it, refreshes it silently,
and lets non-interactive callers supply a token instead.

## Flow

1. Discover endpoints from `<api>/.well-known/oauth-authorization-server`.
2. Register a public client once per API base with `POST /oauth/register`
   (`token_endpoint_auth_method: none`) and cache `client_id` in config.
   Registration requests the scopes `profile.read role.read role.write chat.play`.
3. The provider matches loopback redirect URIs **exactly, including the port**.
   So the client is registered with a fixed list of loopback redirect URIs
   (`http://127.0.0.1:<port>/callback` for ports 41777, 41778, 41779, 41780,
   41781). On login the CLI binds the first free port from that list. If none
   is free it reports which ports are needed instead of guessing.
4. Open the browser at `/oauth/authorize` with `code_challenge` (S256),
   `state`, `resource=<api>/open/v1` and the scopes. `--no-browser` prints the
   URL instead.
5. The loopback handler checks `state`, exchanges the code at `/oauth/token`
   with `code_verifier`, and shows a plain success page.
6. Tokens are stored per API base in `credentials.json` (0600) with expiry.
   Requests refresh with the `refresh_token` when the access token has less
   than two minutes left; a failed refresh asks the user to log in again.
7. `auth logout` revokes the token at `/oauth/revoke` (best effort) and deletes
   the stored entry. `auth status` prints the account from `GET /open/v1/me`.

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
- E2E (env-gated): headless login against a local provider by driving the
  consent endpoints with a session cookie obtained through a seeded email code,
  then `GET /open/v1/me` through the CLI.

## Acceptance

- `hearthroom auth login` on a desktop completes in the browser and
  `hearthroom auth status` shows the account.
- `HEARTHROOM_TOKEN=… hearthroom auth status` works with no config dir.
