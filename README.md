# mon-template-go

A small Go web application template with a standard-library HTTP server,
database-neutral core seams, SQLite/WAL, embedded Goose migrations, manual
dependency injection, tests, and a production container.

## Start

```sh
make run
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

Or run the container:

```sh
docker compose up --build
```

## Layout

- `cmd/` — process entrypoint and signal handling
- `internal/config/` — environment configuration and validation
- `internal/core/domain/` — domain types and errors
- `internal/core/ports/` — narrow capabilities required by core use-cases
- `internal/core/services/` — use-case implementation
- `internal/adapters/httpserver/` — HTTP routes and middleware
- `internal/adapters/stores/sqlite/` — SQLite implementation and migrations
- `internal/di/` — composition root and application lifecycle
- `assets/` — optional frontend sources; no frontend toolchain is imposed

The optional frontend foundation includes neutral, accessible light/dark design
tokens in `assets/src/css/tokens.css`. See `assets/src/css/README.md` for usage
and brand customization; no reset, components, or CSS framework are imposed.

The `/readyz` request is the template's first complete slice:

```text
HTTP adapter -> readiness use-case -> ReadinessStore port -> SQLite adapter
```

## Database portability

SQLite is an adapter, not a core dependency. Core packages do not import
`database/sql`, the SQLite driver, or Goose.

For each new use-case:

1. Define the smallest persistence interface it consumes in
   `internal/core/ports`.
2. Implement that interface in `internal/adapters/stores/sqlite`.
3. Inject it into the use-case from `internal/di`.

To move to PostgreSQL, add `internal/adapters/stores/postgres` implementing the
same core ports, then change the persistence construction in `internal/di`. HTTP
handlers and core use-cases remain unchanged. Keep SQL and migrations in each
database adapter; do not create one large generic repository interface.

SQLite migrations live in `internal/adapters/stores/sqlite/migrations/` and run
when the adapter opens. The in-memory DSN is restricted to one connection so its
schema remains consistent. File databases use WAL, foreign keys, a busy timeout,
and a small connection pool.

## Configuration

| Variable                   | Default  | Purpose                               |
| -------------------------- | -------- | ------------------------------------- |
| `HTTP_ADDR`                | `:8080`  | HTTP listen address                   |
| `DATABASE_DSN`             | `app.db` | SQLite DSN (`DSN` remains a fallback) |
| `HTTP_MAX_HEADER_BYTES`    | `65536`  | Maximum request-header size           |
| `HTTP_READ_HEADER_TIMEOUT` | `5s`     | Request-header deadline               |
| `HTTP_READ_TIMEOUT`        | `15s`    | Full-request read deadline            |
| `HTTP_WRITE_TIMEOUT`       | `30s`    | Response write deadline               |
| `HTTP_IDLE_TIMEOUT`        | `60s`    | Keep-alive idle deadline              |
| `SHUTDOWN_TIMEOUT`         | `10s`    | Graceful shutdown deadline            |

## Commands

```sh
make build
make run
make test
make vet
make lint     # requires golangci-lint
make vuln     # checks reachable known vulnerabilities
```

Tests cover configuration, HTTP health behavior and middleware, core readiness,
dependency wiring, SQLite migrations, connection policy, transactions, and
constraint detection.

## Use as a template

After creating a repository, replace `github.com/esrid/garage` in
`go.mod` and Go imports with your module path, then run:

```sh
go mod tidy
go test ./...
```

Authentication, sessions, CSRF/CORS policy, queues, email, caching, object
storage, payments, observability vendors, and frontend frameworks are
deliberately project-specific.
