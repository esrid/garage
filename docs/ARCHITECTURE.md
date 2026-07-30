# Architecture Guide

**Project Architecture & Development Rules**

> **Goal**
>
> Build a modular monolith that is easy to understand, easy to maintain, and easy
> to scale without introducing unnecessary architectural complexity.
>
> The primary objective is **developer experience**.
>
> A developer should always know where a feature belongs without having to search
> across multiple unrelated directories.

---

## Core Philosophy

The project is organized **by business capability first**, not by technical layer.

This means:

❌

```
handlers/
repositories/
services/
models/
```

❌

```
core/
features/
adapters/
ports/
application/
domain/
```

✅

```
appointment/
customer/
conversation/
followup/
identity/
tenant/
```

Each business capability owns everything related to itself.

The architecture keeps the good parts of Hexagonal Architecture (dependency
inversion, interfaces, separation of concerns) without scattering the code across
many directories.

---

## Folder Structure

```
internal/
│
├── appointment/
│   ├── model.go
│   ├── service.go
│   ├── repository.go
│   ├── postgres.go
│   ├── http.go
│   ├── voice.go
│   ├── view.go
│   ├── page.templ
│   └── *_test.go
│
├── customer/
│
├── vehicle/
│
├── conversation/
│
├── followup/
│
├── identity/
│
├── tenant/
│
├── dashboard/
│
├── site/
│
├── platform/
│   ├── postgres/
│   ├── httpserver/
│   ├── voice/
│   ├── config/
│   ├── sessions/
│   └── logging/
│
└── app/
    ├── app.go
    └── routes.go
```

---

## Philosophy

Every module should be almost completely self-contained.

When working on appointments, everything should live inside:

```
appointment/
```

When working on customers:

```
customer/
```

A developer should never have to ask:

> "Should I modify this inside adapters, features or core?"

The answer should almost always be:

> Go inside the module.

---

## Module Responsibilities

### model.go

Contains business types.

Example:

```go
type Appointment struct {
    ID         string
    TenantID   string
    CustomerID string
}
```

No SQL. No HTTP. No JSON parsing.

### service.go

Contains business logic.

Examples:

- booking appointment
- cancelling appointment
- validation
- transition rules
- quotas
- permissions
- business workflow

The service must never know PostgreSQL, HTTP, JSON, templ, or ElevenLabs.

### repository.go

Defines only the interfaces required by the service.

```go
type Repository interface {
    Book(...)
    Cancel(...)
    FindByID(...)
}
```

Interfaces belong to the consumer. Never create a global `ports/` package.

### postgres.go

Implements repository interfaces.

Contains SQL, transactions, scanning, queries.

Must **not** contain business algorithms. Examples of things that do not belong
here:

❌ `hasCapacity()` · `AvailableSlots()` · `blocksCapacity()`

If the algorithm works without PostgreSQL, it belongs inside the service or
another business file.

### http.go

Contains only HTTP translation:

- parse request
- validate HTTP payload
- call service
- return HTTP response

No business logic.

### voice.go

Same philosophy.

Voice request → business input → call service → voice response. Nothing else.

### page.templ

Presentation only. No business rules.

### view.go

Presentation models. Formatting. Labels. Nothing business critical.

---

## Shared Infrastructure

Everything reused by many modules belongs inside `platform/`:

```
database connection
HTTP server
middleware
configuration
logger
session store
voice authentication
migrations
```

Platform never contains business logic.

---

## Dependency Rules

Business modules must not import one another directly. Instead, declare a local
interface.

`dashboard` needs appointments. Instead of `dashboard → appointment.Service`:

```go
type AppointmentReader interface {
    ListToday(...)
}
```

The DI container injects the implementation.

---

## Flow

```
HTTP → http.go → service.go → repository.go → postgres.go → PostgreSQL

Voice → voice.go → service.go → repository.go → postgres.go
```

---

## Important Rules

**Rule 1** — Organize code by business capability. Never by technical layer.

**Rule 2** — Every new feature belongs to exactly one module.

**Rule 3** — Business rules belong inside the service. Never inside HTTP,
PostgreSQL, templ, JSON or middleware.

**Rule 4** — Repository interfaces belong to the package that consumes them.
Never inside a global package.

**Rule 5** — Handlers translate. Services decide. Repositories persist.

**Rule 6** — PostgreSQL files contain SQL only. No business algorithms.

**Rule 7** — A module never imports another module's internal implementation. If
needed, declare a small local interface.

**Rule 8** — Platform contains reusable infrastructure. Never business logic.

**Rule 9** — Avoid generic repositories unless they genuinely simplify the code.
Business repositories should expose business operations.

Good: `Book()` · `Cancel()` · `ListToday()` · `FindByPhone()`

Bad, when they do not reflect the business: `Update()` · `All()` · `Delete()`

**Rule 10** — Duplicate code is acceptable. Duplicate business rules are not.

**Rule 11** — Never optimize for microservices. Optimize for clarity. If a module
is well isolated, it can always become a microservice later.

**Rule 12** — Do not introduce architecture for architecture's sake. Every
abstraction must remove complexity, never add it.

**Rule 13** — Keep packages small. If a package exceeds roughly one thousand
lines and clearly contains multiple responsibilities, split it. Otherwise, leave
it alone.

**Rule 14** — Do not create sub-packages prematurely. Prefer `appointment/` over
`appointment/{domain,application,adapters,ports}/` until it becomes necessary.

**Rule 15** — One feature. One obvious place. A developer should instinctively
know where new code belongs.

---

## Development Checklist

Before adding code, ask:

| Question | Destination |
|---|---|
| Is this business logic? | `service.go` |
| Is this SQL? | `postgres.go` |
| Is this HTTP translation? | `http.go` |
| Is this voice translation? | `voice.go` |
| Is this presentation? | `page.templ` or `view.go` |
| Is this shared infrastructure? | `platform/` |

If you hesitate between several directories, the architecture is probably wrong.

---

## Success Criteria

A developer unfamiliar with the project should be able to answer:

> "Where is appointment booking implemented?"

In less than 10 seconds. Without using grep. Without searching. Without opening
five directories.

The architecture succeeds when the answer is obvious.

---

## État actuel du dépôt

Ce guide est la **cible**, pas la description du code au 2026-07-30. L'arbre
actuel est `internal/{core,features,adapters,web}` — précisément la disposition
que la section « Core Philosophy » écarte.

L'écart, ce qu'il coûte et le plan de migration par phases sont mesurés dans
[`ARCHITECTURE-AUDIT.md`](ARCHITECTURE-AUDIT.md). Deux constats de cet audit
comptent pour ce guide :

- la **règle 6 est violée aujourd'hui** : `hasCapacity`, `blocksCapacity` et la
  génération de créneaux vivent dans `internal/adapters/stores/postgres/appointment.go` ;
- la **règle 7 est respectée** : aucun module n'importe l'implémentation d'un
  autre, la composition passe déjà par des interfaces déclarées chez le
  consommateur.

Tant que la migration n'est pas faite, le code neuf suit ce guide **dans le
module le plus proche de sa capacité**, et l'audit sert de carte entre les deux
dispositions.

---

## Transition

À lire avant de déplacer le premier fichier. Cette section tranche ce que les
règles ci-dessus laissent ouvert ; sans elle, chaque agent invente sa propre
réponse et la migration diverge.

### Ordre

Le plan par phases est dans [`ARCHITECTURE-AUDIT.md`](ARCHITECTURE-AUDIT.md)
§10. **Phase 0 d'abord** : elle corrige les seuls défauts prouvés — règles
métier coincées dans l'adaptateur SQL, borne de durée écrite quatre fois — sans
déplacer un paquet. `appointment/` passe **en dernier** : deux entrées, le plus
gros module, le plus coûteux à casser.

**Une phase = un commit.** Une phase qui touche plus de trois fichiers de test
hors imports est trop grosse : la couper.

### Les quatre cases que le guide ne remplissait pas

| Ce qui existe | Où ça va | Pourquoi |
|---|---|---|
| `internal/web/{views,page,a11y}` — shell, layout, panel, hero, `Render`, `RequestedDay`, `Origin` | `platform/web/` | partagé par tous les modules et **sans aucune règle métier** : c'est de l'infrastructure de présentation, la règle 8 est respectée |
| `internal/core/domain` — `NormalizePhone`, types d'erreur | `internal/domain/`, **seule exception partagée autorisée** | `NormalizePhone` *est* une règle métier : elle ne peut pas aller dans `platform/` (règle 8), et la dupliquer dans `customer`, `followup` et `tenant` violerait la règle 10. Un module peut donc importer `domain/`, et **rien d'autre** hors de lui-même |
| migrations SQL | `platform/postgres/migrations/` | une base, une séquence numérotée globalement. Le découpage par capacité ne s'applique pas au schéma, même si chaque module garde son propre `postgres.go` |
| `tenant.WithID` / `tenant.IDFromContext` et leur clé de contexte | `internal/domain/` | mesuré : **33 des 51 usages de `tenant` hors du module** ne sont que ces deux fonctions. Laisser tous les modules importer `tenant/` pour ça en referait un `core` déguisé. L'**identifiant** du tenant est ambiant, la **capacité** tenant ne l'est pas : le module `tenant/` garde son service, ses réglages et son quota, et qui en a besoin déclare une interface locale comme n'importe quel autre module |

Aucun autre paquet transverse ne se crée en cours de route. Si un cinquième
candidat apparaît, il se discute avant d'exister.

Concrètement, un module ne peut importer que `internal/domain/` et
`platform/*`. Tout autre import d'un module frère est un défaut de conception,
pas un raccourci.

### Deux pièges mécaniques, déjà payés une fois

**templ** — déplacer un `.templ` ne suffit pas : le `_templ.go` généré reste à
côté de l'ancien emplacement, compile encore, et la page rendue ne correspond
plus au source. Après chaque déplacement de vue : supprimer les `_templ.go`
orphelins, relancer `templ generate`, **et prendre un screenshot**.

**Renommage global d'identifiants** — un `sed` ou un renommage IDE appliqué à
tout l'arbre réécrit aussi les **noms de classes CSS à l'intérieur des chaînes**
(`class="panel-grid"` → `"Panel-grid"`). Le compilateur ne le voit pas, les
tests passent, la page casse. Un outil de renommage doit ignorer tout segment
entre guillemets.

### Ce qui doit être vert avant de passer à la phase suivante

```
go build ./... && go vet ./...
TEST_DATABASE_DSN=<base 18.4 vierge> go test -count=1 -race ./...
```

`TestMigrationsRunOnAVirginDatabase` doit passer : il crée une base réellement
vide et la migre depuis rien. Puis l'application démarre sur cette base et sert
`/`, `/healthz`, `/readyz`, `/login`. Toute vue touchée : screenshot inspecté,
jamais « ça devrait aller ».
