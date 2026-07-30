# WORKBOARD — deux agents

## Règles
- CLAIM avant code.
- Un seul owner par feature.
- Owned paths exclusifs pendant `IN_PROGRESS`.
- Les zones SERIAL exigent un handoff.
- Une feature doit être petite, testable et mergeable.

## Status
`READY` → `CLAIMED` → `IN_PROGRESS` → `REVIEW` → `MERGED`

| ID | Feature | Owner | Depends on | Owned paths | Contract/API frozen | Tests / acceptance | Status |
|---|---|---|---|---|---|---|---|
| F00 | Socle PostgreSQL 18 + pgx + Goose | Agent A | - | `go.mod`, `go.sum`, `Dockerfile`, `compose.yml`, `.github/workflows/ci.yml`, `internal/config/**`, `internal/di/**`, `internal/adapters/stores/postgres/**`, suppression de `internal/adapters/stores/sqlite/**`, `docs/POSTGRESQL.md`, `README.md` | `postgres.Store` satisfait `ports.ReadinessStore`; migrations SQL embarquées | démarrage, migrations idempotentes, readiness, tests PostgreSQL | REVIEW |
| F01 | Tenant + Customer + Vehicle | Agent A | F00 | `internal/core/customer/**`, `internal/core/vehicle/**`, `internal/adapters/stores/postgres/customer*.go`, `internal/adapters/stores/postgres/vehicle*.go`, migrations associées, handlers/services associés | `tenant_id` uniquement depuis contexte serveur; téléphone normalisé; plaque normalisée | create/find by phone, tenant isolation | CLAIMED |
| F02A | Mini-planning atelier — backend | Agent A | F00 | `internal/core/appointment/**`, store PostgreSQL, handlers/services scheduling, migrations associées | scheduling domain API; recheck avant booking; idempotence écriture | disponibilité + création/modification/annulation + tenant isolation | READY |
| F02B | Mini-planning atelier — UI | Agent B | F02A | vues planning, fragments HTMX et styles locaux non globaux | consomme le contrat HTTP figé par F02A | rendu + progressive enhancement + a11y | READY |
| F03 | Voice lookup customer tool | Agent A | F01 | `internal/adapters/voice/**` | webhook/tool schema | known + unknown phone | READY |
| F04 | Dashboard Today | Agent B | F02 | `internal/web/views/**`, `internal/adapters/handlers/dashboard*` | `docs/contracts/F04-dashboard-today.md` (frozen 2026-07-30) | calls/RDV/tasks render | CLAIMED |
| F05 | Voice book appointment | Agent A | F02,F03 | voice tool + booking adapter | `SchedulingProvider.Book` | recheck + idempotency | BLOCKED |
| F06 | CSS tokens + base components | Agent B | - | `assets/src/css/**` | existing token names in `assets/src/css/tokens.css` | responsive/a11y smoke — DONE: light+dark at 360/500/700/1280, no overflow, 3 defects fixed | REVIEW |

Branch `feat/foundation-postgres-css` (pushed) holds F00 + F06 + docs, four
commits, one subject each. Not merged to `main`: that needs the founder's
explicit go-ahead.

Cross-review of F00 by Agent B (AGENTS.md §16) — **passed**. `go build`, `go vet`
and `go test -race ./...` clean. The migration test skips without a database, so
it was run against a live `postgres:18.4-bookworm`: migrations apply and are
idempotent on reopen. The binary was booted against that database and `/healthz`
and `/readyz` both returned 200. Note `compose.yml` maps no port for the
`postgres` service, so the integration test cannot reach it via compose alone —
Agent A's call whether that is intentional.

Notes (Agent B):
- F04/F06 owned paths said `web/...`; no such directory exists. templ output is Go
  code, so views will live in `internal/web/views/` and the dashboard handler in the
  existing `internal/adapters/handlers/`. Paths updated above, no second convention.
- F06 is CSS only. No Go, no new dependency, no DI change — it merges independently
  of F00.
- **Blocked, needs Agent A:** every templ view (F04, F02B) needs
  `github.com/a-h/templ v0.3.1020` in `go.mod` plus one wiring line in the DI root.
  Both are SERIAL zones Agent A holds until F00 is MERGED, so Agent B is not
  touching them. Either add the dependency during F00, or hand `go.mod` over for a
  one-line change once F00 lands. The `GET /app` contract is frozen meanwhile
  (`docs/contracts/F04-dashboard-today.md`), so no UI work is waiting on design.

Notes (Agent A):
- F00 review croisée PASS le 30 juillet 2026, sans finding résiduel.
- Vérifications F00 : `go test -race ./...`, `go vet ./...`, `go build ./...`,
  build Docker, validation Compose, test PostgreSQL 18.4 réel et smoke
  `/healthz` + `/readyz`.
- F00 reste en `REVIEW` jusqu'au commit/merge ; les zones SERIAL restent donc
  détenues par Agent A.

## Open handoff — Agent B to Agent A, 2026-07-30

Read this before claiming anything else.

```
Feature: F06 (done) + unblocking F04/F02B
From: Agent B (frontend)
To: Agent A (backend)

What is pushed: branch feat/foundation-postgres-css, 5 commits, F00 + F06 + docs.
  Nothing is merged to main. main is still at 701371f.

What Agent B needs, and cannot do itself (SERIAL zones you hold):
  1. github.com/a-h/templ v0.3.1020 in go.mod  (verified latest, 2026-05-10)
  2. one wiring line in the DI root for the dashboard handler
  Without both, Agent B can produce zero templ views: F04, F02B, the base
  layout, /static/ serving and htmx are all stalled behind this.

Contracts that MUST NOT change without a mini-task first:
  - docs/contracts/F04-dashboard-today.md — route, Go seam, DTOs, allowed
    status values. tenant_id stays out of the seam: context only.
  - assets/src/css/tokens.css token names. Add tokens, do not rename them.

Two findings from the F00 cross-review, your call:
  - compose.yml exposes no port for the postgres service, so the integration
    test cannot reach it through compose alone. Reviewed with a throwaway
    container instead. Intentional or an oversight?
  - When serving CSS, embed the directory: //go:embed assets/src/css.
    app.css @imports tokens.css as a sibling URL, so embedding app.css alone
    404s on tokens.css and the page renders unstyled with no error. Noted in
    assets/src/css/README.md.

Known limitations of F06:
  - styleguide.html hand-codes the dashboard markup that F04 will rebuild in
    templ. Delete the panel-grid section from the styleguide once the templ
    dashboard exists, so there is one source of markup.
  - Keyboard traversal and a real screen reader are untested: both need a
    served page.
  - No brand palette. The template's neutral accent is still in place and is
    marked [À VALIDER].

Tests run: go build, go vet, go test -race ./... all clean. Migration test run
  against a live postgres:18.4-bookworm (it skips without TEST_DATABASE_DSN).
  Binary booted against it: /healthz and /readyz both 200. CSS verified in
  headless Chrome, light and dark, at 360/500/700/1280.

Next safe task for Agent A: F01 or F02A — neither touches Agent B's paths
  (assets/src/css/**, internal/web/views/**, internal/adapters/handlers/dashboard*).
```

## SERIAL zones
Current owner must be written here before edits.

| Zone | Owner | Until | Reason |
|---|---|---|---|
| `go.mod` / `go.sum` | Agent A | F00 MERGED | migration SQLite vers pgx/Goose |
| DI root | Agent A | F00 MERGED | wiring PostgreSQL |
| DB migration numbering | Agent A | F02A MERGED | schéma backend initial |
| `compose.yml` | Agent A | F00 MERGED | service PostgreSQL 18 |
| `Dockerfile` | Agent A | F00 MERGED | supprimer les hypothèses SQLite de l'image applicative |
| global layout/tokens | Agent B | F06 MERGED | global UI contract |
| provider interfaces | - | - | cross-feature contract |

## Handoff template
```
Feature:
From:
To:
What is merged:
Contracts that MUST NOT change:
Known limitations:
Tests run:
Next safe task:
```
