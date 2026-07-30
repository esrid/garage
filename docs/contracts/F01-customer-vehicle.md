# Contract — F01 Tenant, Customer and Vehicle

**Frozen on:** 2026-07-30 by Agent A (backend).
**Consumers:** F03 voice lookup and future authenticated HTTP handlers.

Changing a signature, normalization rule or tenant boundary below requires an
explicit mini-task in `WORKBOARD.md` before implementation.

## Tenant boundary

Tenant-scoped service methods never accept `tenant_id` in an input struct or as
a free argument. The authenticated server boundary puts the tenant ID in the Go
context and services read it with:

```go
tenant.WithID(ctx context.Context, tenantID string) context.Context
tenant.IDFromContext(ctx context.Context) (string, error)
```

An absent tenant ID is an authorization error. Stores receive the resolved ID
from their service and include it in every scoped SQL predicate. Store methods
are internal capabilities; they are never exposed directly to an LLM or HTTP
payload.

## Tenant service

Tenant creation is the unscoped onboarding operation:

```go
tenant.Service.Create(ctx context.Context, input tenant.CreateInput) (tenant.Tenant, error)
```

`CreateInput` contains `Name` and `Timezone`. Timezone defaults to
`America/Martinique` when empty. This operation returns the database-generated
tenant ID; callers cannot choose it.

## Customer service

```go
customer.Service.Create(ctx context.Context, input customer.CreateInput) (customer.Customer, error)
customer.Service.FindByPhone(ctx context.Context, phone string) (customer.Customer, error)
```

`CreateInput` contains `FirstName`, `LastName` and `Phone`. The returned model
contains `ID`, `TenantID`, normalized `Phone`, names and timestamps.

Phone rules:

- storage and lookup use E.164-like canonical form: `+` followed by 8–15 digits;
- spaces, dots, hyphens and parentheses are discarded;
- an international `00` prefix becomes `+`;
- local/national numbers are rejected rather than assigned an invented country;
- uniqueness is `(tenant_id, normalized_phone)`.

## Vehicle service

```go
vehicle.Service.Create(ctx context.Context, input vehicle.CreateInput) (vehicle.Vehicle, error)
vehicle.Service.FindByPlate(ctx context.Context, plate string) (vehicle.Vehicle, error)
vehicle.Service.ListByCustomer(ctx context.Context, customerID string) ([]vehicle.Vehicle, error)
```

`CreateInput` contains `CustomerID`, `Plate`, `Make` and `Model`. Plate is
optional at creation because the caller may not have confirmed it yet. A lookup
requires a non-empty valid plate.

Plate rules:

- canonical lookup removes spaces and hyphens and uppercases ASCII letters;
- only 2–15 ASCII letters/digits are accepted after normalization;
- the original trimmed plate remains available for display;
- uniqueness applies to non-empty plates within one tenant;
- no registration, make or model is inferred.

The database enforces that `(tenant_id, customer_id)` references a customer in
the same tenant. A caller cannot attach a vehicle to another tenant's customer.

## Errors

- invalid input: `domain.ValidationError`;
- missing tenant context: `domain.UnauthorizedError`;
- missing customer/vehicle: `domain.NotFoundError`;
- duplicate phone or plate in one tenant: `domain.AlreadyExistsError`;
- unexpected persistence errors are wrapped with operation context and do not
  expose the database DSN.

## HTTP

F01 defines no HTTP route. No current frontend feature consumes customer or
vehicle endpoints. Before adding one, create and freeze a separate contract with
method, route, request payload, response shape, error statuses and authentication
requirements.

## Acceptance

- the same phone can exist in two tenants and resolves independently;
- a phone present only in tenant A is not visible from tenant B;
- a customer ID from tenant A cannot be used to create a vehicle in tenant B;
- phone and plate normalization are deterministic and tested;
- the service API contains no caller-controlled tenant ID.

## Official references

Verified on 2026-07-30 before the first F01 database implementation:

- [pgxpool v5.10.0 `Query`, `QueryRow` and `Exec`](https://pkg.go.dev/github.com/jackc/pgx/v5@v5.10.0/pgxpool);
- [pgx v5 `PgError`](https://pkg.go.dev/github.com/jackc/pgx/v5/pgconn);
- [PostgreSQL 18 UUID type and native generators](https://www.postgresql.org/docs/18/datatype-uuid.html);
- [PostgreSQL 18 constraints, including composite foreign keys](https://www.postgresql.org/docs/18/ddl-constraints.html);
- [PostgreSQL 18 `INSERT ... RETURNING`](https://www.postgresql.org/docs/18/sql-insert.html);
- [PostgreSQL 18 unique indexes and NULL semantics](https://www.postgresql.org/docs/18/indexes-unique.html).

The Go standard-library `time/tzdata` package documentation was also verified
on 2026-07-30. It is imported by `cmd/main.go`, as recommended for applications,
so IANA timezone validation remains reliable in the minimal Alpine image.
