# Contract — F09 Staff Authentication

**Frozen on:** 2026-07-30 by Agent A (backend).
**Consumers:** future login UI, all existing `/app` handlers.

Changing a route, form field, cookie property, session lifetime or tenant
boundary below requires an explicit mini-task in `WORKBOARD.md` first.

## Boundary and identity

- A staff email resolves its tenant on the server. No authentication route,
  form, cookie or service login input accepts `tenant_id`.
- One normalized email belongs to one tenant in the MVP. Multi-tenant staff
  membership is out of scope and requires a new contract.
- After a session is verified, middleware places both the staff identity and
  its persisted tenant ID in `context.Context`.
- All `/app` routes, including reads and appointment mutations, require that
  context. Voice tool Bearer authentication and public/health/static routes are
  separate and unchanged.

## HTTP routes

Bodies use `application/x-www-form-urlencoded` for native HTML forms.

| Method / route | Input | Success |
|---|---|---|
| `POST /auth/login` | `email`, `password`, optional `next` | `303 See Other` to validated local `next`, otherwise `/app`, plus session cookie |
| `POST /auth/logout` | none | session revoked, cookie expired, `303 See Other` to `/` |

There is deliberately no backend-owned `GET` login page. The frontend may add
one without changing this contract.

### Error behavior

| Condition | Status |
|---|---:|
| wrong email/password, disabled user, locked account | `401` |
| missing/invalid form or unsupported media type | `422` / `415` |
| login derivation budget exhausted | `429` plus `Retry-After` |
| cross-origin unsafe browser request | `403` |
| database unavailable | `503` |

Authentication failures always use the same public message and never reveal
whether an email exists, is disabled, or is temporarily locked. Responses are
`Cache-Control: no-store`. Request bodies are bounded.

## Session contract

- Cookie name: `__Host-garage_session`.
- Attributes: `Secure`, `HttpOnly`, `SameSite=Strict`, `Path=/`; no `Domain`.
- Lifetime: 12 hours, enforced by PostgreSQL and the cookie expiry.
- The cookie contains 32 random bytes encoded as unpadded base64url.
- PostgreSQL stores only a SHA-256 digest of that token, never the bearer token.
- Login always creates a fresh token. Logout deletes the current server session
  and expires the cookie; repeating logout remains safe.
- Missing, malformed, expired or revoked sessions are indistinguishable to the
  caller.

Unsafe browser requests pass through Go's `http.CrossOriginProtection` in
addition to `SameSite=Strict`. Safe HTTP methods never mutate state.

### Amendment 2026-07-30 — F12 browser re-authentication

When a session is missing, malformed, expired or revoked, the `/app` middleware
selects one response by request type:

| Request | Response |
|---|---|
| HTMX (`HX-Request: true`) | `401` plus `HX-Redirect: /login` |
| HTML navigation (`Sec-Fetch-Mode: navigate` or `Accept: text/html`) | `303` to `/login?next=<original app path and query>` |
| other client | `401` text response |

HTMX takes precedence when both its header and `Accept: text/html` are present.
The `next` value is derived only from the current local `/app` request URI and
query-encoded by the server; no caller-supplied return URL is accepted. A store
failure remains `503` and never masquerades as an expired session.

`HX-Redirect` is intentionally carried on the `401`, not a 3xx: HTMX's official
documentation states that it does not process response headers on 3xx responses.
See [HTMX `HX-Redirect`](https://htmx.org/headers/hx-redirect/) (verified
2026-07-30). The frontend owns `GET /login`; F12 did not change
`POST /auth/login`.

### Amendment 2026-07-30 — F16 browser login outcomes

`POST /auth/login` distinguishes an HTML form navigation from a non-browser API
client. For a failed HTML navigation (`Sec-Fetch-Mode: navigate` or
`Accept: text/html`) it returns `303` to `/login?error=<code>`; the closed codes
are `invalid`, `rate_limited`, `unavailable`, and `forbidden`. Wrong
credentials, invalid/missing form data and unsupported media type all use
`invalid`, preserving account-enumeration resistance. `429` sets `Retry-After`
before the redirect. Cross-origin protection still runs outside this handler
and may return its direct `403` response.

For `HX-Request: true`, which takes precedence over the HTML headers, a failed
login returns `401` plus `HX-Redirect` to the same local login error URL. Other
clients retain the original `401`, `415`, `422`, `429`, or `503` status and
bounded text response. Error redirects never echo email, password, provider
details, or an arbitrary return URL.

On success, optional form field `next` is honored only when it parses as a
relative request URI whose path is exactly `/app` or begins `/app/`, with no
fragment, userinfo, host, scheme, backslash or control character. Invalid or
absent values fall back to `/app`; they are never repaired into a redirect.

## Password contract

- Passwords are stored as versioned PBKDF2-HMAC-SHA-256 hashes using 600,000
  iterations, a random 16-byte salt and a 32-byte derived key.
- Raw passwords are never persisted or logged.
- Staff provisioning requires 12–128 bytes. Login rejects larger inputs before
  expensive derivation.
- Hash comparison is constant-time. An unknown email still performs one
  derivation so the response does not trivially enumerate accounts by timing.
- Five consecutive failures lock the account for 15 minutes. Success clears the
  counter. The public response stays identical while locked.
- To bound CPU use from varied unknown emails, one application process accepts
  at most 30 password derivations per one-minute window and runs at most
  two simultaneously. Excess valid login forms fail before PBKDF2 with `429`.
  This deliberately process-local limit matches the single-instance MVP; a
  multi-instance deployment must replace or complement it at the trusted edge.

Staff provisioning is a backend service for the onboarding feature; F09 does
not invent a public signup or tenant-selection endpoint.

## Explicit amendments to frozen consumers

- **F02A:** its three `/app` mutation routes are now guarded by a server session
  and cross-origin protection. They remain tenant-scoped through context.
- **F04:** `GET /app` and `GET /app/today` now return `401` before reaching the
  dashboard provider when the session is absent or invalid. This replaces the
  former unauthenticated degraded `200` behavior.

## Acceptance

- login for a valid member establishes only that member's tenant context;
- another tenant's data remains inaccessible even if object IDs are known;
- wrong and unknown credentials have identical public outcomes;
- expired/revoked/malformed sessions cannot reach an `/app` handler;
- logout revokes the server session;
- cross-origin browser POST requests are rejected;
- migrations and store tests pass against PostgreSQL 18.x.
