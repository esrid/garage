# Assistant de réception atelier IA

Monolithe Go garage-first avec serveur `net/http`, PostgreSQL 18, pgx,
migrations Goose embarquées et injection de dépendances manuelle.

## Démarrage

Avec Docker Compose :

```sh
export POSTGRES_PASSWORD='local-password'
export DATABASE_DSN='postgres://garage:local-password@postgres:5432/garage?sslmode=disable'
docker compose up --build
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

Pour exécuter le binaire localement, démarrer PostgreSQL puis fournir la chaîne
de connexion :

```sh
export DATABASE_DSN='postgres://garage:password@localhost:5432/garage?sslmode=disable'
make run
```

## Architecture backend

- `cmd/` — démarrage du processus et arrêt ordonné ;
- `internal/config/` — configuration depuis l'environnement ;
- `internal/core/domain/` — types et erreurs du domaine ;
- `internal/core/ports/` — capacités étroites requises par les cas d'usage ;
- `internal/core/services/` — logique applicative ;
- `internal/adapters/httpserver/` — routes `net/http` et middleware ;
- `internal/adapters/stores/postgres/` — stores et migrations PostgreSQL ;
- `internal/di/` — composition root.

Le domaine ne dépend ni de pgx ni de Goose. Les stores PostgreSQL implémentent
les ports définis par les cas d'usage. Garder le SQL lisible et éviter un
repository générique.

Le contrat de readiness actuel est :

```text
GET /readyz -> readiness service -> ReadinessStore.Ping -> pgxpool.Ping
```

Les migrations SQL sont embarquées dans le binaire et exécutées au démarrage.
Les versions, responsabilités et références officielles sont documentées dans
[`docs/POSTGRESQL.md`](docs/POSTGRESQL.md).

## Configuration

| Variable | Défaut | Responsabilité |
|---|---:|---|
| `HTTP_ADDR` | `:8080` | adresse d'écoute HTTP |
| `DATABASE_DSN` | requis | chaîne de connexion PostgreSQL (`DSN` reste un fallback) |
| `HTTP_MAX_HEADER_BYTES` | `65536` | taille maximale des headers |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | délai de lecture des headers |
| `HTTP_READ_TIMEOUT` | `15s` | délai de lecture de la requête |
| `HTTP_WRITE_TIMEOUT` | `30s` | délai d'écriture de la réponse |
| `HTTP_IDLE_TIMEOUT` | `60s` | délai keep-alive |
| `SHUTDOWN_TIMEOUT` | `10s` | délai d'arrêt ordonné |

## Commandes

```sh
make build
make test
make vet
make lint
```

Les tests d'intégration PostgreSQL utilisent `TEST_DATABASE_DSN`. Ils sont
ignorés quand la variable n'est pas définie.
