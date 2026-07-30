# Scheduling

Verified on 2026-07-30 against PostgreSQL 18, pgx v5.10.0 and Go 1.26.5.

## Responsibility

F02A provides an internal workshop schedule behind `SchedulingProvider`. It
stores explicit opening windows, computes candidate slots, and performs booking,
rescheduling and cancellation. It does not invent business hours, diagnose a
vehicle or integrate Google Calendar.

The frozen consumer contract is
[`contracts/F02A-planning.md`](contracts/F02A-planning.md).

## Concurrency choice

Availability reads are advisory. A write is confirmed only after a second
capacity check in a PostgreSQL transaction.

The transaction selects the concrete opening window with `FOR UPDATE`. All
writes competing inside that opening therefore serialize on the same row. After
obtaining the lock, the provider counts overlapping active appointments and
only writes when capacity remains. This uses ordinary Read Committed semantics:
the count statement runs after the waiting row lock is obtained and sees the
concurrent transaction that just committed.

No Redis lock, external queue or PostgreSQL extension is required.

Day reads resolve the supplied instant in the persisted tenant timezone. An
opening that crosses midnight is clipped to the selected tenant calendar day,
and appointments that began the previous day still consume overlapping
capacity.

## Idempotency

Every write requires an opaque idempotency key. A tenant-scoped command row is
claimed inside the same transaction before the appointment mutation. The unique
key makes a concurrent duplicate wait for the first transaction. A matching
retry returns the complete first result stored as validated PostgreSQL `jsonb`;
the same key with a different operation/request returns a conflict.

## HTTP

Mutation handlers use native `net/http` forms. They call `ParseForm` explicitly
and use `PostForm`, never `FormValue`, because `FormValue` discards parsing
errors. Bodies are bounded before parsing. Appointment IDs come from Go 1.22+
ServeMux wildcards through `Request.PathValue`.

## Official references

- [pgx v5 transactions and `BeginTxFunc`](https://pkg.go.dev/github.com/jackc/pgx/v5@v5.10.0)
- [PostgreSQL 18 `SELECT ... FOR UPDATE`](https://www.postgresql.org/docs/18/sql-select.html#SQL-FOR-UPDATE-SHARE)
- [PostgreSQL 18 Read Committed isolation](https://www.postgresql.org/docs/18/transaction-iso.html)
- [PostgreSQL 18 JSON types](https://www.postgresql.org/docs/18/datatype-json.html)
- [Go 1.26.5 `Request.ParseForm` and `PathValue`](https://pkg.go.dev/net/http@go1.26.5)
- [Go 1.26.5 `encoding/json`](https://pkg.go.dev/encoding/json@go1.26.5)
