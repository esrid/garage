# AGENTS.md — Assistant de réception atelier IA

## Mission
Construire un SaaS simple, rentable et maintenable par un solopreneur. Le premier vertical commercial est le garage indépendant/petit-moyen atelier en Martinique. Le coeur produit doit rester générique, mais l'UX et les workflows du MVP sont garage-first.

## North Star
Une feature est utile si elle améliore au moins un de ces points :
1. démo vendable ;
2. valeur métier du garage ;
3. fiabilité/sécurité ;
4. marge/coût ;
5. simplicité de maintenance.
Sinon, ne pas la construire maintenant.

## Stack figée
- Go + `net/http`
- PostgreSQL 18.x + pgx
- Goose migrations
- templ pour SSR
- HTML natif moderne
- CSS natif + design tokens
- HTMX comme progressive enhancement, jamais comme moteur du domaine
- Stimulus seulement quand du JS local suffisamment structuré le justifie
- Docker Compose / Dokploy / VPS
- ElevenLabs pour le MVP voix, derrière `VoiceProvider`
- Planning interne derrière `SchedulingProvider`; Google Calendar est optionnel
- Stripe Checkout / Customer Portal lorsque le paiement est branché

## Interdits par défaut
Ne pas introduire sans justification écrite : React/Next.js, Tailwind, Redis, Kafka, Kubernetes, microservices, ORM généraliste, queue externe, state manager frontend, framework HTTP Go, moteur de workflow complexe.

## Ordre de préférence frontend
Avant d'implémenter un comportement UI :
1. vérifier HTML natif (`dialog`, `details`, `summary`, forms, popover, etc.) ;
2. vérifier CSS natif moderne ;
3. utiliser HTMX si interaction serveur ;
4. utiliser quelques lignes de JS natif si interaction purement locale ;
5. utiliser Stimulus si le comportement JS mérite une structure ;
6. seulement ensuite envisager une dépendance.

## Architecture
Monolithe modulaire. Pas de Ports & Adapters cérémoniel partout.
Les interfaces provider sont justifiées pour les dépendances externes coûteuses/changeantes :
- `VoiceProvider`
- `SchedulingProvider`
- éventuellement `BusinessEnrichmentProvider`

Le domaine ne dépend jamais directement du SDK d'un provider.

## Invariants métier
- `tenant_id` vient du contexte serveur, jamais d'un argument libre envoyé par le LLM.
- Aucune donnée d'un tenant ne doit être accessible à un autre tenant.
- Un RDV n'est confirmé oralement qu'après succès du backend/provider.
- Prix, statut, disponibilité et données véhicule ne sont jamais inventés.
- Toute écriture externe importante doit être idempotente.
- L'agent qualifie une panne, il ne pose pas de diagnostic mécanique certain.

## Documentation d'abord
Pour toute API/librairie externe : lire la documentation officielle actuelle avant d'implémenter. Ne jamais inventer un endpoint, un champ ou une option depuis la mémoire.

Toute nouvelle dépendance doit documenter :
- problème concret ;
- pourquoi stdlib/HTML/CSS/HTMX ne suffit pas ;
- coût/risque de maintenance ;
- version pinée ;
- lien vers documentation officielle.

Toute intégration externe doit avoir une page `docs/<integration>.md` décrivant responsabilité, auth, endpoints, webhooks, erreurs, coûts/quotas et liens officiels.

## Travail parallèle à deux agents
Le fichier `docs/WORKBOARD.md` est la source de vérité.

### Avant de coder
Chaque agent doit CLAIM une feature et renseigner :
- ID ;
- objectif utilisateur ;
- owner ;
- dépendances ;
- chemins de fichiers possédés ;
- contrats à respecter ;
- tests attendus.

### Ownership
Un agent ne modifie pas les chemins possédés par l'autre sans handoff explicite.
Zones SERIAL (un seul agent à la fois) :
- `go.mod` / `go.sum`
- racine DI/wiring
- migrations concurrentes touchant les mêmes tables
- `compose.yml`
- layout global
- tokens CSS globaux
- interfaces provider partagées

### Contrats avant parallélisme
Si A et B dépendent l'un de l'autre, figer d'abord le contrat minimal : interface Go, endpoint HTTP, schéma DB ou fragment/DTO. Ensuite chaque agent peut travailler indépendamment. Ne pas changer ce contrat silencieusement.

### Branches
- `feat/Fxx-description`
- branches courtes ; idéalement merge le jour même
- jamais deux énormes branches qui divergent plusieurs jours
- après un merge, mettre sa branche à jour depuis `main` avant de toucher une zone partagée

### Review croisée
L'agent qui n'a pas écrit la feature fait la première review. Vérifier :
- invariants métier ;
- simplicité ;
- docs officielles ;
- tenant isolation ;
- erreurs ;
- tests ;
- dépendances ;
- lisibilité pour un humain.

Ne pas refactoriser « au passage » le travail de l'autre.

## Definition of Done
Une feature n'est DONE que si :
- résultat utilisateur démontrable ;
- tests pertinents passent ;
- erreurs traitées explicitement ;
- isolation tenant vérifiée si applicable ;
- docs mises à jour ;
- aucune dépendance injustifiée ;
- review croisée terminée ;
- branche mergeable sans TODO critique caché.

## Produit garage MVP
P0 :
- appel réel ;
- identification client par téléphone ;
- fiches client/véhicule ;
- collecte/confirmation plaque ;
- qualification demande ;
- mini-planning atelier ;
- création/modification/annulation RDV ;
- statut intervention simple ;
- demande de rappel/devis ;
- transfert humain ;
- historique/transcript/résumé ;
- dashboard du jour ;
- usage/cost metering.

Non-P0 : ERP, stock universel, facturation garage, diagnostic mécanique IA, multi-sites complexe, workflow builder, microservices.

## Règle économique
Pas d'illimité. Tant que les factures réelles ne prouvent pas mieux, utiliser 0,10 €/minute comme budget prudent voix/IA. Mesurer coût par tenant et par conversation.

## Style de code
- explicite > clever
- petits packages cohérents
- petites interfaces orientées besoin, pas repository géant
- SQL lisible
- commentaires pour le pourquoi, pas pour paraphraser le code
- erreurs avec contexte
- aucune abstraction « au cas où »


## Standard Library First

Before creating any helper, abstraction, middleware, parser, validator, data structure, utility, or dependency, first verify whether the functionality already exists in:

1. The Go standard library.
2. The official Go extended packages under `golang.org/x/...`.
3. An existing package already used by this project.
4. The official documentation of the external service being integrated.

Do not rely only on memory. Check the current official documentation before implementing.

Examples of things that must be checked before being reimplemented:

* HTTP routing and middleware support
* URL and query parsing
* JSON encoding and decoding
* Form parsing
* Cookies
* Context cancellation and timeouts
* Cryptography and hashing
* Random token generation
* Email and MIME parsing
* Validation primitives
* Filesystem operations
* Structured logging
* Rate limiting
* Concurrency primitives
* String, slice, map, time, and networking utilities
* HTML features such as `dialog`, `details`, `summary`, popovers, native validation, and semantic form elements
* Modern CSS features such as grid, flexbox, custom properties, container queries, `:has()`, nesting, layers, and responsive functions

The order of preference is:

```text
Existing browser or HTML capability
→ Go standard library
→ golang.org/x package
→ existing project dependency
→ small project-specific implementation
→ new third-party dependency
```

Do not reinvent functionality that the platform already provides.

Do not create a generic abstraction around a standard-library feature unless the project has a concrete need for it.

## HTTP Router Organization

Do not register every route inside one enormous root `http.ServeMux`.

Organize routes by feature using subrouters or feature-level registration functions.

Preferred structure:

```go
func NewRouter(deps Dependencies) http.Handler {
    root := http.NewServeMux()

    root.Handle("/customers/", customers.NewHandler(deps.Customers))
    root.Handle("/appointments/", appointments.NewHandler(deps.Appointments))
    root.Handle("/vehicles/", vehicles.NewHandler(deps.Vehicles))
    root.Handle("/calls/", calls.NewHandler(deps.Calls))

    return root
}
```

Each feature should own and register its own routes:

```go
func NewHandler(service *Service) http.Handler {
    mux := http.NewServeMux()

    mux.HandleFunc("GET /customers", service.List)
    mux.HandleFunc("POST /customers", service.Create)
    mux.HandleFunc("GET /customers/{id}", service.Show)

    return mux
}
```

A feature package should ideally contain its own:

```text
handler.go
routes.go
service.go
queries.go
templates/
tests/
```

The root router should only compose feature routers and global middleware.

Do not place business logic inside route registration.

Do not create a custom router framework. Use `net/http` composition unless the standard library demonstrably cannot satisfy the requirement.

## Mandatory Pre-Implementation Check

Before writing code, briefly answer internally:

1. Does Go already provide this?
2. Does `golang.org/x` already provide this?
3. Does HTML or CSS already provide this?
4. Does the project already contain this functionality?
5. Does the official API or library already expose the required operation?
6. Can this be implemented with a small, explicit function instead of a new abstraction?
7. Which feature owns this route and these files?

Only proceed after this check.

When completing a task, mention in the implementation summary which official documentation or standard-library capability was checked.

