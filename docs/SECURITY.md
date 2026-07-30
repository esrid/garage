# Backend security

Last verified against the official documentation on **2026-07-30**.

## Staff authentication

F09 uses only the Go standard library for password derivation, random tokens,
constant-time comparison, cookies and cross-origin request protection. It adds
no authentication framework and no client-side token storage.

- Passwords: PBKDF2-HMAC-SHA-256 with 600,000 iterations, a unique random salt
  and a versioned encoded format. OWASP prefers Argon2id, but its documented
  PBKDF2-HMAC-SHA-256 work factor is used here to preserve the project's
  standard-library-first rule and avoid a security-critical dependency.
- Sessions: opaque random cookie value; only its SHA-256 digest is persisted.
  Cookies are `Secure`, `HttpOnly`, `SameSite=Strict`, host-only and scoped to
  `/`. Session-bearing responses are not cacheable.
- CSRF: Go `http.CrossOriginProtection` rejects unsafe cross-origin browser
  requests. `SameSite=Strict` is defense in depth, not the only check.
- Tenant isolation: login resolves tenant membership from the persisted staff
  row. `tenant_id` never comes from the login request or cookie payload.
- Brute force: five consecutive failures produce a persisted 15-minute account
  lock. The response does not disclose account existence or state. A separate
  process-wide budget allows 30 password derivations per minute and two at once,
  so varied unknown emails cannot freely saturate the VPS CPU. `429` responses
  include `Retry-After`.

Operational requirements:

- Serve production traffic only over HTTPS. The session cookie is intentionally
  unusable over plain HTTP.
- Never log form bodies, passwords, session cookie values or database DSNs.
- Database backups contain password hashes and active session digests and must
  receive the same access controls as production.
- Expired session rows may be removed by an operational maintenance job; expiry
  is enforced during every lookup even before cleanup.

Official references:

- [Go `crypto/pbkdf2`](https://pkg.go.dev/crypto/pbkdf2)
- [Go `net/http.CrossOriginProtection`](https://pkg.go.dev/net/http#CrossOriginProtection)
- [Go `net/http.Cookie`](https://pkg.go.dev/net/http#Cookie)
- [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
