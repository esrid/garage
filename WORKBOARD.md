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
| F00 | Socle PostgreSQL 18 + pgx + Goose | Agent A | - | `go.mod`, `go.sum`, `Dockerfile`, `compose.yml`, `.github/workflows/ci.yml`, `internal/config/**`, `internal/di/**`, `internal/adapters/stores/postgres/**`, suppression de `internal/adapters/stores/sqlite/**`, `docs/POSTGRESQL.md`, `README.md` | `postgres.Store` satisfait `ports.ReadinessStore`; migrations SQL embarquées | démarrage, migrations idempotentes, readiness, tests PostgreSQL | MERGED |
| F01 | Tenant + Customer + Vehicle | Agent A | F00 | `docs/contracts/F01-customer-vehicle.md`, `docs/DATABASE.md`, `internal/core/tenant/**`, `internal/core/customer/**`, `internal/core/vehicle/**`, `internal/adapters/stores/postgres/tenant*.go`, `internal/adapters/stores/postgres/customer*.go`, `internal/adapters/stores/postgres/vehicle*.go`, `internal/adapters/stores/postgres/migrations/00002_tenant_customer_vehicle.sql`, `cmd/main.go` (import stdlib `time/tzdata` uniquement) | `docs/contracts/F01-customer-vehicle.md` (frozen 2026-07-30); `tenant_id` uniquement depuis contexte serveur | create/find by phone, tenant isolation | REVIEW |
| F02A | Mini-planning atelier — backend | Agent A | F00 | `internal/core/appointment/**`, store PostgreSQL, handlers/services scheduling, migrations associées | scheduling domain API; recheck avant booking; idempotence écriture | disponibilité + création/modification/annulation + tenant isolation | READY |
| F02B | Mini-planning atelier — UI | Agent B | F02A | vues planning, fragments HTMX et styles locaux non globaux | consomme le contrat HTTP figé par F02A | rendu + progressive enhancement + a11y | READY |
| F03 | Voice lookup customer tool | Agent A | F01 | `internal/adapters/voice/**` | webhook/tool schema | known + unknown phone | READY |
| F04 | Dashboard Today | Agent B | F02A | `internal/web/views/**`, `internal/adapters/handlers/dashboard*` | `docs/contracts/F04-dashboard-today.md` (frozen 2026-07-30) | calls/RDV/tasks render | REVIEW — page servie sur `GET /app`, fragment `GET /app/today`, 11 tests, vérifiée en navigateur à 380 et 1280 px |
| F05 | Voice book appointment | Agent A | F02,F03 | voice tool + booking adapter | `SchedulingProvider.Book` | recheck + idempotency | BLOCKED |
| F06 | CSS tokens + base components | Agent B | - | `assets/src/css/**` | existing token names in `assets/src/css/tokens.css` | responsive/a11y smoke — DONE: light+dark at 360/500/700/1280, no overflow, 3 defects fixed | MERGED |

PR #1 was merged by the founder as `229598e` on `main`. The local
`feat/foundation-postgres-css` HEAD is the second parent of that merge and has no
content diff with `origin/main`. Agent A and Agent B continue stacking focused
commits on the feature branch; neither agent merges to `main`.

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
- F00 was merged by the founder in PR #1 (`229598e`).

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

## Handoff response — Agent A to Agent B, 2026-07-30

Contexte autonome : réponse au handoff F06/F04 ci-dessus, écrite avant le code
F01. Agent A a relu AGENTS.md, tout WORKBOARD.md, le PRD V3 et tous les contrats
gelés dans `docs/contracts/`.

- Review F00 : résultat PASS accepté. Le fondateur a depuis mergé la PR #1 sous
  `229598e` ; aucun autre merge vers `main` ne sera fait par Agent A.
- Port PostgreSQL Compose : absence d'exposition hôte **intentionnelle** pour le
  service applicatif par défaut. La base reste accessible à `app` sur le réseau
  Compose sans publier 5432. Les tests d'intégration passent via le service
  PostgreSQL 18.4 de CI (`TEST_DATABASE_DSN`) ou un conteneur local temporaire
  avec un port explicite. Ne pas ouvrir le port de production par défaut.
- Static CSS : finding accepté. La future mini-tâche de static serving devra
  embarquer **tout le répertoire** `assets/src/css`, pas seulement `app.css`, et
  préserver les URLs sœurs afin que `@import "tokens.css"` ne retourne jamais
  404. Le placement exact du `//go:embed` respectera la règle Go des chemins
  relatifs au package qui porte la directive.
- templ `v0.3.1020` : ajout autorisé reconnu ; Agent A ne le retirera pas.
- Wiring dashboard : Agent A conserve la zone SERIAL DI. Le câblage sera une
  mini-tâche ciblée dès que le constructeur du handler F04 appartenant à Agent B
  existe. Aucun type, route ou DTO de `docs/contracts/F04-dashboard-today.md` ne
  sera modifié silencieusement.
- F01 est maintenant `IN_PROGRESS`. Ses chemins n'intersectent aucun owned path
  Agent B. F02A ne commencera qu'après passage de F01 en `REVIEW`.

## Open handoff — Agent A to Agent B, F01 review, 2026-07-30

Contexte autonome pour une review sans historique préalable :

```
Feature: F01 Tenant + Customer + Vehicle
From: Agent A (backend)
To: Agent B (frontend/reviewer)

Status: REVIEW. Aucun merge vers main demandé ou effectué.

Contract frozen first:
  docs/contracts/F01-customer-vehicle.md

Owned implementation to review:
  docs/DATABASE.md
  cmd/main.go                         (un seul import stdlib time/tzdata)
  internal/core/tenant/**
  internal/core/customer/**
  internal/core/vehicle/**
  internal/adapters/stores/postgres/tenant.go
  internal/adapters/stores/postgres/customer.go
  internal/adapters/stores/postgres/vehicle.go
  internal/adapters/stores/postgres/customer_vehicle_test.go
  internal/adapters/stores/postgres/migrations/00002_tenant_customer_vehicle.sql

Contracts/invariants:
  - aucun service Customer/Vehicle n'accepte tenant_id ; il vient du context ;
  - chaque SELECT/INSERT scoped inclut tenant_id ;
  - FK composite (tenant_id, customer_id) empêche un véhicule cross-tenant ;
  - téléphone +596... et plaque ont une clé normalisée tenant-scopée ;
  - aucun endpoint HTTP inventé : F03 consommera les services Go ;
  - IDs générés par PostgreSQL 18 uuidv7(), timezone IANA embarquée via stdlib.

Review indépendante déjà faite : un finding moyen et un faible ont été corrigés
(NotFound sur ListByCustomer cross-tenant ; deux véhicules NULL plate testés),
puis re-review PASS sans finding résiduel.

Tests run:
  go test -race ./...
  go vet ./...
  go build ./...
  docker build -t garage-f01-test .
  TEST_DATABASE_DSN=... go test -count=1 -race -v \
    ./internal/adapters/stores/postgres ./internal/di
  PostgreSQL réel : postgres:18.4-bookworm ; aucun test d'intégration skippé.

Acceptance demonstrated:
  même téléphone/plaque dans deux tenants ; téléphone privé invisible ;
  duplicate intra-tenant mappé AlreadyExists ; rattachement véhicule cross-tenant
  rejeté NotFound ; plusieurs véhicules sans plaque acceptés.

Review demand:
  vérifier tenant isolation, contrat, SQL/migration backward-safe, erreurs,
  simplicité et lisibilité. Écrire findings/PASS dans WORKBOARD seulement.

Next safe Agent A task while review runs: freeze F02A HTTP/domain contract.
No F01 code will be changed silently after this handoff.
```

## Handoff 2 — Agent B to Agent A, 2026-07-30 (after F04)

```
Feature: F04 Dashboard Today — code complet, en attente de données réelles
From: Agent B (frontend)
To: Agent A (backend)

CE QUI EST LIVRÉ ET VÉRIFIÉ
  GET /app        page complète du jour (appels / RDV / à traiter)
  GET /app/today  le même fragment de panneaux, pour le rafraîchissement htmx
  GET /static/    assets embarqués (CSS + htmx 2.0.10 auto-hébergé)
  Vérifié en navigateur : 1280 px et 380 px, aucun débordement. HTML complet
  servi sans JavaScript. 11 tests sur le handler et les vues.

CE QUE TU DOIS FAIRE, ET C'EST LA SEULE CHOSE QUI MANQUE
  internal/adapters/httpserver/handler.go contient :
      dashboard := handlers.NewDashboard(handlers.FixtureToday{})
  FixtureToday est de la donnée de démonstration. Quand F02A existe :
    1. écris un adaptateur qui satisfait handlers.TodayProvider :
         Today(ctx context.Context, day time.Time) (views.Today, error)
       tenant_id vient de ctx, PAS d'un paramètre. La signature rend
       l'erreur impossible, ne l'élargis pas.
    2. injecte-le depuis le DI root et supprime
       internal/adapters/handlers/dashboard_fixture.go
  Rien d'autre à toucher : ni les vues, ni le CSS, ni les routes.

POURQUOI LA CONSTRUCTION EST DANS LE ROUTER ET PAS DANS LE DI
  Changer la signature de httpserver.New() aurait touché le DI root pendant que
  tu étais dans F01. Déplace-la dans le DI quand tu câbles le vrai provider :
  c'est sa place, je ne l'y ai pas mise pour ne pas te bloquer.

CE QUE LE HANDLER GARANTIT, NE LE CASSE PAS
  - provider en erreur => page dégradée avec un message, jamais un 500 ni une
    page blanche. Ne renvoie pas d'erreur en pensant que l'UI fera un 500.
  - statut inconnu => badge neutre + valeur brute affichée. Tu peux ajouter des
    statuts sans toucher au CSS ; ils s'afficheront en clair jusqu'à ce que
    j'ajoute leur libellé français.
  - valeur vide => tiret cadratin. Plaque, véhicule, prix : jamais inventés.
  - Cache-Control: no-store sur les deux routes (donnée opérationnelle).

CONTRAT AMENDÉ (avant que tu écrives quoi que ce soit contre lui)
  docs/contracts/F04-dashboard-today.md, section "Amendment 2026-07-30" :
  les DTO sont dans internal/web/views/today.go et non dans handlers, et une
  ligne retombe sur le numéro de téléphone quand le nom est inconnu.

CE QUI RESTE CHEZ MOI, ET CE QUI ME BLOQUE
  - F02B (planning UI) : bloqué, j'attends que tu gèles le contrat HTTP de F02A.
    Gèle-le dans docs/contracts/ avant de coder, comme tu l'as fait pour F01.
  - F07 (site public + SEO, PRD §11) : non commencé, ne dépend de rien chez toi.
  - Parcours clavier et lecteur d'écran réels : toujours non testés.
  - Aucune palette de marque : l'accent neutre du template est en place,
    marqué [À VALIDER] par le fondateur.

Tests lancés : go build, go vet, go test -race ./... tous verts, y compris tes
  paquets F01. Migrations vérifiées contre postgres:18.4-bookworm réel.
```

## SERIAL zones
Current owner must be written here before edits.

| Zone | Owner | Until | Reason |
|---|---|---|---|
| `go.mod` / `go.sum` | Agent A | F00 MERGED | migration SQLite vers pgx/Goose |
| `go.mod` / `go.sum` — ajout templ | Agent B — **RELEASED** | fait le 2026-07-30 | `github.com/a-h/templ v0.3.1020` ajouté sur autorisation explicite du fondateur, agent A absent. Une seule dépendance, ancrée par `internal/web/views/layout.templ` (sinon `go mod tidy` la supprime). Rien d'autre touché : DI root et routes intacts. |
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
