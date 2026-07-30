# PostgreSQL 18, pgx v5 et Goose v3

Vérifié le 30 juillet 2026 à partir des documentations et dépôts officiels.

## Responsabilité

Ce socle fournit :

- un PostgreSQL persistant pour le monolithe ;
- un pool `pgxpool.Pool` partagé par les stores de l'application ;
- l'exécution au démarrage des migrations SQL embarquées ;
- un contrôle de disponibilité réel de la base pour la readiness.

Il ne définit aucun contrat métier, endpoint HTTP ou schéma applicatif. Les signatures de `ports.ReadinessStore` et des handlers doivent être figées par l'owner backend avant qu'un autre module ne les consomme.

## Versions retenues

| Composant | Version à pinner | Justification |
|---|---:|---|
| PostgreSQL | `18.4` | Mineure courante de PostgreSQL 18, version supportée. Le projet PostgreSQL recommande d'exécuter la mineure courante d'une majeure. |
| Image Docker | `postgres:18.4-bookworm` | Tag officiel qui fixe la mineure PostgreSQL et la distribution de base. Un digest doit être ajouté au déploiement si une image strictement immuable est requise ; ne pas inventer ce digest, le résoudre pour l'image effectivement déployée. |
| pgx | `github.com/jackc/pgx/v5 v5.10.0` | Dernier tag stable v5 vérifié. `v5` est la majeure stable actuelle de pgx. |
| Goose | `github.com/pressly/goose/v3 v3.27.3` | Dernière release stable v3 vérifiée. Son `go.mod` exige Go 1.25.7 ; le toolchain local vérifié est Go 1.26.5. |

Sources : [politique de versions PostgreSQL](https://www.postgresql.org/support/versioning/), [tags officiels de l'image](https://github.com/docker-library/official-images/blob/master/library/postgres), [tags pgx](https://github.com/jackc/pgx/tags), [release Goose v3.27.3](https://github.com/pressly/goose/releases/tag/v3.27.3).

## Pourquoi ces dépendances

PostgreSQL est un service externe : la bibliothèque standard seule ne fournit pas de driver PostgreSQL. `pgx` est le driver imposé par la stack et `pgxpool` fournit son pool concurrent. Goose apporte l'ordre, l'historique et l'exécution reproductible des changements de schéma ; les réimplémenter localement serait une responsabilité sensible sans valeur produit.

Le coût de maintenance est limité à deux modules Go versionnés, mais leurs mises à jour doivent être suivies. Le raccord entre l'API native pgx et Goose passe explicitement par `database/sql`, et les migrations concurrentes demandent une stratégie explicite. Aucun ORM ni second driver PostgreSQL n'est nécessaire.

## Connexion et pool pgx

Le chemin documenté est :

1. analyser la chaîne de connexion avec `pgxpool.ParseConfig` si des paramètres de pool doivent être fixés explicitement ;
2. construire le pool avec `pgxpool.NewWithConfig(ctx, config)` ;
3. appeler immédiatement `pool.Ping(ctxAvecTimeout)` avant de déclarer l'application prête ;
4. fermer le pool avec `pool.Close()` lors de l'arrêt ordonné.

`pgxpool.New` et `pgxpool.NewWithConfig` retournent sans attendre qu'une connexion soit établie. Leur succès ne prouve donc pas que le serveur, le réseau ou les identifiants fonctionnent. `Pool.Ping` acquiert une connexion et exécute une instruction vide ; c'est la vérification adaptée au démarrage et à la readiness. Utiliser un contexte borné pour ne jamais bloquer indéfiniment un démarrage ou une requête de readiness.

La chaîne de connexion peut être au format URL ou mots-clés PostgreSQL. Elle contient potentiellement un secret : ne jamais la journaliser, même lorsqu'une erreur de parsing ou de connexion est enrichie avec du contexte.

Sources : [documentation `pgxpool`](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool), [README officiel pgx](https://github.com/jackc/pgx).

## Goose Provider et adaptateur `stdlib`

Le `Provider` Goose v3 a cette dépendance publique :

```go
goose.NewProvider(dialect goose.Dialect, db *sql.DB, fsys fs.FS, opts ...goose.ProviderOption)
```

Il n'accepte pas directement un `*pgxpool.Pool`. Avec le Provider et pgx, le package `github.com/jackc/pgx/v5/stdlib` est donc nécessaire comme couche de compatibilité `database/sql`.

Le raccord recommandé est `stdlib.OpenDBFromPool(pool)`. Il réutilise le pool pgx existant, règle automatiquement à zéro le nombre maximal de connexions inactives du wrapper `*sql.DB` afin de ne pas affamer les utilisateurs directs du pool, et n'ouvre pas un second pool applicatif indépendant. Fermer le `*sql.DB` retourné ne ferme pas le `*pgxpool.Pool`.

Comme le code appelle directement `stdlib.OpenDBFromPool`, un import anonyme destiné uniquement à enregistrer le nom de driver n'est pas requis. L'alternative `sql.Open("pgx", dsn)` utilise bien l'enregistrement du driver `stdlib`, mais créerait une gestion de pool `database/sql` séparée ; elle n'est pas retenue ici.

Attention au cycle de vie : `Provider.Close()` ferme le `*sql.DB` qui lui a été fourni, mais, dans ce montage, cela ne ferme toujours pas le pool pgx sous-jacent. Le propriétaire du pool reste responsable de `pool.Close()`.

Sources : [API `pgx/v5/stdlib`](https://pkg.go.dev/github.com/jackc/pgx/v5/stdlib), [documentation du Provider Goose](https://pressly.github.io/goose/documentation/provider/), [API Goose v3](https://pkg.go.dev/github.com/pressly/goose/v3).

## Migrations SQL embarquées

Le Provider accepte directement un `fs.FS`. Le montage sans état global est :

```go
//go:embed migrations/*.sql
var embeddedMigrations embed.FS

migrationsFS, err := fs.Sub(embeddedMigrations, "migrations")
if err != nil {
    // retourner une erreur enrichie
}

migrationDB := stdlib.OpenDBFromPool(pool)
provider, err := goose.NewProvider(
    goose.DialectPostgres,
    migrationDB,
    migrationsFS,
)
if err != nil {
    // retourner une erreur enrichie
}

_, err = provider.Up(ctx)
```

`goose.SetBaseFS` existe pour l'ancienne API globale (`goose.UpContext`, etc.), mais il est inutile avec `NewProvider`, qui reçoit le filesystem explicitement. Ne pas mélanger les deux styles.

`Provider.Up(ctx)` applique seulement les migrations en attente. S'il n'y en a aucune, il retourne une liste vide et une erreur `nil` : relancer l'application ne réapplique donc pas les migrations déjà enregistrées. Cette idempotence d'orchestration ne dispense pas d'écrire chaque migration avec une stratégie de reprise et de compatibilité appropriée.

Par défaut, Goose ne verrouille pas l'exécution entre plusieurs processus. Pour le MVP à une seule instance, exécuter les migrations avant de servir le trafic. Avant tout déploiement à plusieurs réplicas, choisir explicitement soit une étape de migration sérialisée, soit le mécanisme de verrouillage Goose documenté. Ne pas inventer une configuration de lock au moment du scaling : vérifier alors l'API de la version pinée.

Source : [Provider Goose et migrations embarquées](https://pressly.github.io/goose/documentation/provider/).

## Health et readiness

Trois contrôles ont des responsabilités différentes :

| Contrôle | Vérification | Échec attendu |
|---|---|---|
| Démarrage application | création du pool, `pool.Ping` borné, puis `provider.Up` | ne pas ouvrir le serveur HTTP ; quitter avec une erreur contextualisée sans secret |
| Liveness HTTP | processus HTTP vivant | ne doit pas dépendre d'une panne PostgreSQL passagère, afin d'éviter une boucle de redémarrage |
| Readiness HTTP | `pool.Ping` avec délai court | statut non prêt, typiquement HTTP 503 ; ne pas exposer le DSN ni le détail interne de l'erreur |

Les chemins HTTP et la signature exacte de `ports.ReadinessStore` ne sont pas définis ici. Ils doivent être figés comme contrat backend avant implémentation ou consommation par un autre agent.

Dans Compose, le healthcheck du conteneur PostgreSQL peut utiliser l'outil officiel :

```yaml
healthcheck:
  test: ["CMD-SHELL", "pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}"]
  interval: 10s
  timeout: 5s
  retries: 5
  start_period: 10s
```

`pg_isready` retourne 0 lorsque le serveur accepte les connexions, 1 lorsqu'il les rejette pendant le démarrage, 2 sans réponse et 3 si aucun test n'a été tenté. Il n'est pas nécessaire de fournir des identifiants corrects pour obtenir l'état du serveur : ce healthcheck prouve que PostgreSQL accepte des connexions, pas que les identifiants de l'application sont valides. Le `pool.Ping` applicatif reste obligatoire.

Compose peut attendre ce healthcheck avec `depends_on: condition: service_healthy`. Cela ordonne le démarrage local, mais ne remplace ni les erreurs explicites au démarrage ni la readiness de l'application.

Sources : [`pg_isready` PostgreSQL 18](https://www.postgresql.org/docs/18/app-pg-isready.html), [ordre de démarrage Compose](https://docs.docker.com/compose/how-tos/startup-order/), [`Pool.Ping`](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool#Pool.Ping).

La CI utilise également l'image PostgreSQL 18.4 comme service, avec un
healthcheck `pg_isready`, et fournit `TEST_DATABASE_DSN` à la suite Go. Les tests
d'intégration ne sont ainsi pas silencieusement ignorés dans la validation de la
branche. Ce montage suit la documentation officielle des
[service containers PostgreSQL de GitHub Actions](https://docs.github.com/en/actions/tutorials/use-containerized-services/create-postgresql-service-containers).

## Image PostgreSQL 18

Pour PostgreSQL 18 et suivants, l'image officielle a changé son organisation : `PGDATA` vaut `/var/lib/postgresql/18/docker` et le `VOLUME` déclaré est `/var/lib/postgresql`. Le volume Compose doit donc cibler `/var/lib/postgresql`, pas l'ancien chemin `/var/lib/postgresql/data` utilisé jusqu'à PostgreSQL 17.

`POSTGRES_PASSWORD` est la seule variable obligatoire de l'image ; `POSTGRES_USER` et `POSTGRES_DB` sont optionnelles. Ces variables d'initialisation n'ont d'effet que si le répertoire de données est vide. Changer leur valeur après la première initialisation ne modifie pas les rôles ou bases déjà persistés. Les secrets ne doivent pas être commités dans Compose.

Le tag `postgres:18.4-bookworm` est disponible officiellement à la date de vérification. Les tags sont mutables ; pour une reproductibilité binaire stricte en production, compléter le tag par le digest réellement résolu dans le pipeline de déploiement.

Sources : [documentation de l'image Docker officielle](https://github.com/docker-library/docs/blob/master/postgres/README.md), [source de vérité des tags officiels](https://github.com/docker-library/official-images/blob/master/library/postgres).

## Erreurs et points à ne pas inventer

- Ne pas considérer la création du pool comme un test de connexion : appeler `Ping`.
- Ne pas passer un `*pgxpool.Pool` à `goose.NewProvider` : l'API exige `*sql.DB`.
- Ne pas ajouter `lib/pq` : `pgx/v5/stdlib` fournit déjà la compatibilité attendue.
- Ne pas utiliser simultanément l'API globale `SetBaseFS` et le `Provider` local.
- Ne pas annoncer les migrations comme sûres entre plusieurs processus sans sérialisation ou locker explicitement configuré.
- Ne pas utiliser l'ancien montage de volume PostgreSQL 17 avec l'image 18.
- Ne pas figer ici un endpoint, une signature `ReadinessStore`, des tailles de pool, des délais ou un digest d'image non encore contractés et vérifiés.

## Références officielles

- [PostgreSQL 18 — notes de la version majeure](https://www.postgresql.org/docs/18/release-18.html)
- [PostgreSQL 18.4 — notes de version](https://www.postgresql.org/docs/release/18.4/)
- [PostgreSQL — politique de versions](https://www.postgresql.org/support/versioning/)
- [PostgreSQL 18 — `pg_isready`](https://www.postgresql.org/docs/18/app-pg-isready.html)
- [Docker Official Image — PostgreSQL](https://github.com/docker-library/docs/blob/master/postgres/README.md)
- [Docker Official Images — tags PostgreSQL](https://github.com/docker-library/official-images/blob/master/library/postgres)
- [pgx v5 — dépôt officiel](https://github.com/jackc/pgx)
- [pgxpool — API officielle](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool)
- [pgx stdlib — API officielle](https://pkg.go.dev/github.com/jackc/pgx/v5/stdlib)
- [Goose Provider — documentation officielle](https://pressly.github.io/goose/documentation/provider/)
- [Goose v3 — API officielle](https://pkg.go.dev/github.com/pressly/goose/v3)
- [Goose v3.27.3 — release officielle](https://github.com/pressly/goose/releases/tag/v3.27.3)
- [Docker Compose — ordre de démarrage](https://docs.docker.com/compose/how-tos/startup-order/)
