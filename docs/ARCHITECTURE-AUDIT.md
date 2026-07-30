# Audit d'architecture — 2026-07-30

Aucun code n'a été modifié pour cet audit. Toutes les affirmations ci-dessous
viennent de `go list`, de `grep` sur l'arbre courant et de la lecture des
fichiers, pas d'une impression.

Commit analysé : `1e21dad`. Toolchain Go 1.27rc2.

---

## 1. Flux complet d'une réservation

Le rendez-vous a **deux points d'entrée** qui convergent sur le même service.
C'est l'observation la plus importante de cet audit : la règle métier n'est
écrite qu'une fois, mais elle est **atteinte par deux chemins qui valident
séparément**.

### 1.1 Entrée comptoir — `POST /app/appointments`

| Étape | Fichier | Rôle |
|---|---|---|
| Montage de la route | `internal/adapters/httpserver/handler.go` | compose la frontière `/app`, applique `requireStaffSession` |
| Session → contexte | `internal/adapters/httpserver/middleware.go` | pose l'identité staff et le `tenant_id` dans le `context.Context` |
| Enregistrement du motif | `internal/features/planning/appointments.go` (`Register`) | `POST /app/appointments` |
| Traduction HTTP | `internal/features/planning/appointments.go` (`Book`) | `parseAppointmentForm`, `parseStart`, `parseDuration`, puis `303` ou code d'erreur fermé |
| Règle métier | `internal/core/appointment/service.go` (`Book`) | tenant obligatoire, UUID, `validateWrite` (label, durée, note, clé d'idempotence) |
| Contrat de sortie | `internal/core/appointment/appointment.go` | `BookInput`, `SchedulingProvider` |
| Implémentation | `internal/adapters/stores/postgres/appointment.go` (`Book`) | transaction, `lockOpening` (`FOR UPDATE`), recontrôle de capacité, rejeu idempotent |
| Schéma | `internal/adapters/stores/postgres/migrations/00003_appointment.sql` | tables `appointments`, `appointment_commands`, FK composites |
| Câblage | `internal/di/di.go` | `appointment.NewService(database, database, database, database)` |

### 1.2 Entrée vocale — `POST /voice/tools/appointment-book`

| Étape | Fichier | Rôle |
|---|---|---|
| Montage | `internal/adapters/httpserver/handler.go` (`mountVoiceTools`) | frontière `/voice/` |
| Credential → contexte | `internal/adapters/voice/auth.go`, `internal/adapters/voice/toolrequest.go` | Bearer par tenant, `DecodeToolRequest` pose le tenant dans le contexte |
| Traduction JSON | `internal/features/voicetools/appointment_booking.go` (`Book`) | parse RFC3339, durée, dérive la clé d'idempotence depuis `conversation_id` |
| **Même service** | `internal/core/appointment/service.go` (`Book`) | identique au comptoir |
| **Même implémentation** | `internal/adapters/stores/postgres/appointment.go` | identique |

**Fichiers impliqués dans les deux flux réunis : 11.**

```
cmd/main.go
internal/di/di.go
internal/adapters/httpserver/handler.go
internal/adapters/httpserver/middleware.go
internal/adapters/voice/auth.go
internal/adapters/voice/toolrequest.go
internal/features/planning/appointments.go
internal/features/voicetools/appointment_booking.go
internal/core/appointment/appointment.go
internal/core/appointment/service.go
internal/adapters/stores/postgres/appointment.go
```

---

## 2. Qui porte des règles, qui ne fait que traduire

### Porte des règles métier

| Package | Règles qu'il détient |
|---|---|
| `core/appointment` | table de transitions, bornes de durée, validation d'écriture, ce qui est une réservation valide |
| `core/auth` | politique de mot de passe, verrouillage après 5 échecs, durée de session |
| `core/customer`, `core/vehicle` | normalisation téléphone et plaque, unicité par tenant |
| `core/conversation` | seuils d'alerte 70/85/100, arrondi minute supérieure, cohérence d'un événement |
| `core/followup` | ce qu'est une demande en attente, refus d'une ligne d'un autre tenant |
| `core/tenant` | bornes du quota, normalisation du numéro de transfert |
| `core/domain` | `NormalizePhone`, types d'erreur partagés |
| **`adapters/stores/postgres`** | **⚠️ génération de créneaux, `hasCapacity` (pic de concurrence), `blocksCapacity` (quels statuts consomment), pas de 15 minutes** |

### Ne fait que traduire

| Package | Traduit |
|---|---|
| `adapters/httpserver` | motifs de routes, frontières de confiance, middlewares |
| `adapters/voice` | credential et préambule de requête |
| `features/*` | formulaire ⇄ DTO, code d'erreur ⇄ phrase française, rendu |
| `web/views` | shell, composants, libellés |
| `web/page` | `Render`, `RequestedDay`, `Origin` |

### Zone grise assumée

`features/planning/view.go` dérive la **clé d'idempotence** (`sha256(opération |
id | updated_at | durée)`). C'est une décision sur la sémantique d'une écriture,
posée dans une couche de présentation. Documenté comme tel dans le code, mais
c'est une règle, pas un affichage.

---

## 3. Duplication réelle

### 3.1 La borne de durée, écrite trois fois — duplication confirmée

| Endroit | Code |
|---|---|
| `core/appointment/service.go:152` | `validateDuration` → « must be 15 to 480 » |
| `features/planning/appointments.go:145` | `minutes < 15 \|\| minutes > 480 \|\| minutes%15 != 0` |
| `features/voicetools/appointment_booking.go:191` | `minutes < 15 \|\| minutes > 480 \|\| minutes%15 != 0` |
| `features/planning/view.go:91` | `PlanningDurations = []int{15, 30, 45, 60, 90, 120, 180, 240}` |

Les deux adaptateurs revalident ce que le service valide déjà. Ce n'est pas
absurde — ils veulent un message d'erreur dans leur propre enveloppe — mais la
constante `480` vit à quatre endroits. Changer la borne demande quatre éditions,
et rien ne casse si on en oublie une : le service refusera, avec un message que
l'adaptateur n'avait pas prévu.

**Correctif possible sans réorganisation** : exporter
`appointment.DurationBounds()` / `appointment.ValidDurations()` et faire lire les
trois autres.

### 3.2 La capacité et les créneaux vivent dans le store — pas une duplication, un mauvais étage

`internal/adapters/stores/postgres/appointment.go` contient :

- `AvailableSlots` : la boucle qui avance de 15 minutes dans une ouverture
  (`ligne 75`) ;
- `hasCapacity` (`ligne 451`) : calcul du **pic de concurrence** par balayage
  d'événements ;
- `blocksCapacity` (`ligne 447`) : quels statuts consomment de la capacité.

Aucune de ces trois n'a besoin de SQL. Ce sont les règles les plus subtiles du
produit, elles sont testées via des tests d'intégration PostgreSQL, et elles sont
inaccessibles à un test unitaire du domaine. **C'est le vrai défaut structurel de
cet audit**, plus que les noms de dossiers.

### 3.3 La table de transitions, lue deux fois

`core/appointment` expose `CanTransition` / `NextStatuses`. Le store
(`UpdateAppointmentStatus`) **reconstruit la liste des statuts sources** en
appelant `CanTransition` dans une boucle pour bâtir le `ANY($4)` du `UPDATE`.
La règle reste unique — le store la consomme, il ne la réécrit pas — mais la
mécanique est subtile et mérite d'être nommée dans le domaine
(`appointment.SourcesFor(to)`).

### 3.4 Ce qui n'est **pas** dupliqué

`features/planning` et `features/voicetools` ne partagent aucune règle : ils
parsent des formats différents (formulaire vs JSON) et appellent le même service.
La convergence est réelle et vérifiée au §1.

---

## 4. Interfaces des services du coeur

20 interfaces, **toutes déclarées dans le package qui les consomme**, aucune dans
un package `ports` central (celui-ci a été supprimé le 2026-07-30 précisément
pour cette raison).

| Interface | Déclarée dans | Implémentée par |
|---|---|---|
| `appointment.SchedulingProvider` | `core/appointment/appointment.go:100` | `postgres.Store` |
| `appointment.DayReader` | `core/appointment/appointment.go:107` | `postgres.Store` |
| `appointment.OpeningConfigurer` | `core/appointment/appointment.go:111` | `postgres.Store` |
| `appointment.StatusUpdater` | `core/appointment/appointment.go:122` | `postgres.Store` (assertion `appointment.go:483`) |
| `auth.Store` | `core/auth/auth.go:77` | `postgres.Store` (`auth.go:15`) |
| `conversation.Store` | `core/conversation/service.go:18` | `postgres.Store` (`conversation.go:16`) |
| `conversation.HistoryStore` | `core/conversation/read.go:16` | `postgres.Store` (`conversation.go:17`) |
| `conversation.HistoryReader` | `core/conversation/read.go:22` | `conversation.HistoryService` |
| `conversation.UsageStore` | `core/conversation/usage.go:38` | `postgres.Store` (`conversation_usage.go:11`) |
| `conversation.UsageReader` | `core/conversation/usage.go:43` | `conversation.UsageService` |
| `customer.Store` | `core/customer/service.go:10` | `postgres.Store` (`customer.go:14`) |
| `customer.ReadStore` | `core/customer/read.go:51` | `postgres.Store` (`customer_read.go:14`) |
| `customer.FileReader` | `core/customer/read.go:57` | `customer.ReadService` |
| `followup.Store` | `core/followup/service.go:16` | `postgres.Store` (`followup.go:12`) |
| `followup.ReadStore` | `core/followup/read.go:18` | `postgres.Store` (`followup_read.go:10`) |
| `followup.PendingReader` | `core/followup/read.go:24` | `followup.ReadService` |
| `followup.CallerDirectory` | `core/followup/read.go:88` | `followup.ReadService` |
| `tenant.Store` | `core/tenant/service.go:26` | `postgres.Store` (`tenant.go:14`) |
| `vehicle.Store` | `core/vehicle/service.go:11` | `postgres.Store` (`vehicle.go:14`) |
| `services.ReadinessStore` | `core/services/readiness.go:16` | `postgres.Store` |

Trois interfaces consommées **côté adaptateur** et non par le coeur :
`handlers`-level `PlanningReader` (`features/planning`), `CallHistoryReader`
(`features/dashboard`), `TodayProvider` (`features/dashboard`). Elles sont
déclarées là où elles sont consommées, même règle.

**Observation** : `postgres.Store` implémente **11 interfaces**. C'est un objet
unique qui satisfait tout le monde. Pratique au câblage
(`appointment.NewService(database, database, database, database)`), mais ce
quadruple `database` est le symptôme visible d'un store monolithique.

---

## 5. Violations de direction

Mesuré avec `go list -deps`, pas à l'œil.

| Règle | Résultat |
|---|---|
| `core` importe `adapters` | **0** |
| `core` importe `features` | **0** |
| `core` importe `net/http`, `pgx` ou `templ` | **0** |
| `features` importe une autre `features` | **0** |
| `web/views` importe `features` ou `core` | **0** (aucun import interne) |

Une seule feature compose un domaine voisin : `features/dashboard` consomme le
modèle de lecture des appels et des rappels — via des interfaces déclarées chez
lui, pas par un import de `features/calls`.

### Ce qui viole l'esprit sans violer la lettre

1. **PostgreSQL détient des règles** (§3.2). Le code ne « fuit » pas vers le
   coeur : c'est l'inverse, du métier est resté dans l'adaptateur.
2. **Un handler dérive une clé d'idempotence** (`features/planning/view.go`).
3. **Trois adaptateurs revalident les bornes du domaine** (§3.1).

Aucun handler ne contient de règle d'autorisation ou de transition : celles-ci
sont bien dans `core` et dans les contraintes SQL.

---

## 6. Ce qui est difficile à suivre

Une même feature métier traverse plusieurs répertoires. Mesuré sur « réserver un
rendez-vous » : **5 fichiers dans 4 répertoires** (§1).

| Difficulté | Détail |
|---|---|
| **`adapters/stores/postgres`** | 11 fichiers non-test, **1 657 lignes**, un seul type `Store` pour 7 domaines. C'est le package le plus lourd et le plus couplé du dépôt. |
| **`features/planning`** | 795 lignes : page, fragment, mutations RDV, mutations ouverture, DTO, vues. Trois responsabilités sous un même toit. |
| **Domaine ↔ feature aux noms différents** | `core/appointment` → `features/planning` ; `core/conversation` → `features/calls`, `features/usage`, `features/postcall` ; `core/auth` → `features/identity`. Correct sur le fond (une feature est un écran, pas une table), mais il faut connaître la correspondance. |

À l'inverse, ce qui est facile : ajouter une page (une ligne dans la feature),
ajouter un outil voix (le préambule est partagé), changer un libellé (un seul
endroit).

---

## 7. Réutilisable vs couplé

| Package | Verdict |
|---|---|
| `core/domain` | **réutilisable** — types d'erreur et normalisation téléphone, aucune dépendance |
| `core/tenant` | **réutilisable** — le contexte tenant est un motif générique |
| `core/auth` | **réutilisable** — PBKDF2 stdlib, sessions, verrouillage ; rien de spécifique au garage |
| `core/customer`, `core/vehicle` | semi — E.164 générique, plaque française spécifique |
| `core/appointment` | **couplé au métier** — c'est le produit |
| `core/conversation` | **couplé à ElevenLabs** dans sa forme, pas dans son interface |
| `web/page`, `web/views` | **réutilisable** dans un autre projet Go+templ ; les libellés sont français et garage |
| `adapters/voice` | couplé à la forme « tool ElevenLabs », mais le préambule bearer+JSON est générique |
| `adapters/stores/postgres` | **fortement couplé** — un `Store` qui connaît 7 domaines |
| `features/*` | **couplés par nature**, c'est leur rôle |

---

## 8. Risque d'une réorganisation par module métier

### Déplaçable sans risque

- `core/auth` → `modules/identity/` : consommé par 2 endroits, interfaces nettes.
- `core/conversation` + `features/{calls,usage,postcall}` → `modules/calls/` :
  déjà cohérent, aucun import croisé.
- `core/followup` + sa part de `postgres` → `modules/followups/`.
- Les fichiers `postgres/<domaine>*.go` se découpent proprement : ils sont déjà
  nommés par domaine.

### Cycles d'import probables

| Risque | Cause |
|---|---|
| **Élevé** | `modules/dashboard` compose rendez-vous + appels + rappels. S'il importe les trois modules et qu'un seul le réimporte, cycle. Aujourd'hui évité par des interfaces déclarées chez le consommateur : **il faut garder cette règle**. |
| **Élevé** | `postgres.Store` unique : le découper par module oblige à choisir entre un pool partagé injecté (facile) et des `Store` par module (implique de dupliquer `Open`, les migrations et le pool). |
| **Moyen** | `core/tenant` est importé par 8 packages. Il devient un `shared/tenant`, sinon tout module l'importe et il redevient un `core` déguisé. |
| **Faible** | `web/views` est déjà sans import interne. |

### Tests qui casseront

- **Tous les tests d'intégration PostgreSQL** (7 fichiers) : ils ouvrent
  `postgres.Open` et appellent les services. Un découpage du store change leurs
  imports, pas leur logique.
- `internal/web/a11y` : importe 6 features par leur chemin. Renommage direct.
- `internal/adapters/httpserver` : le test d'inventaire des routes construit
  `Deps` avec 13 champs. Un champ renommé = une ligne.
- Les tests unitaires de domaine (`core/*`) : **aucun impact** s'ils déménagent
  avec leur package.

Estimation : **~15 fichiers de test touchés, aucun réécrit** — ce sont des
changements d'import.

### Câblage DI à refaire

`internal/di/di.go` est aujourd'hui **une fonction de 60 lignes** qui construit
23 objets. Une réorganisation par module la remplace par N appels
`modules/<x>.Wire(pool)`. C'est le fichier le plus impacté, et le seul dont la
réécriture est structurelle plutôt que mécanique.

---

## 9. Deux options

### Option A — garder la structure, corriger ce qui est mesurément faux

| | |
|---|---|
| **Coût** | 1 à 2 jours |
| **Risque** | faible : aucun déplacement de package |
| **Bénéfice** | supprime les 3 défauts réels du §3 et du §5 |

Contenu :

1. **Remonter les règles de capacité dans le domaine** — `hasCapacity`,
   `blocksCapacity` et la génération de créneaux passent de
   `adapters/stores/postgres` à `core/appointment`. Le store lit la journée, le
   domaine calcule. Débloque des tests unitaires sur la règle la plus subtile du
   produit. **C'est le point le plus rentable de tout cet audit.**
2. **Une seule source pour les bornes de durée** — `appointment.ValidDurations()`
   consommé par les deux adaptateurs et la vue.
3. **`appointment.SourcesFor(status)`** pour que le store ne rebâtisse plus la
   liste inverse.
4. **Déplacer la dérivation de la clé d'idempotence** de la vue vers le domaine.
5. Découper `features/planning` en trois fichiers déjà nommés (`page`,
   `appointments`, `openings` — c'est déjà le cas) et documenter la
   correspondance domaine ↔ feature dans `AGENTS.md`.

### Option B — modules métier, progressivement

| | |
|---|---|
| **Coût** | 4 à 6 jours, étalés |
| **Risque** | moyen : cycles d'import et découpage du store |
| **Bénéfice** | une feature = un dossier ; le §6 disparaît |

À ne **pas** faire d'un bloc : le dernier découpage (features) a pris une
demi-journée et a produit deux collisions de noms et un renommage global qui a
réécrit des classes CSS dans des chaînes. Un découpage plus profond, en une fois,
produira pire.

---

## 10. Arbre cible proposé et plan par phases

### Arbre cible

**L'arbre cible est celui de [`ARCHITECTURE.md`](ARCHITECTURE.md)**, arbitré par
le fondateur le 2026-07-30. Il n'est pas répété ici : deux arbres cibles dans un
même dépôt sont exactement la confusion que cette réorganisation cherche à
supprimer.

En résumé : `internal/<capacité>/` à plat, chacune portant `model.go`,
`service.go`, `repository.go`, `postgres.go`, `http.go`, `voice.go`, `view.go`,
`page.templ` et ses tests ; l'infrastructure partagée dans `platform/` ; la
composition et les routes dans `app/`.

Cet arbre est **plus plat** que celui que cet audit proposait initialement
(`modules/` + `shared/`) : pas de niveau intermédiaire, et le nom du répertoire
est le nom de la capacité. Les trois règles qui le maintiennent en vie sont
inchangées et deviennent les règles 6, 7 et 8 du guide :

1. un module n'importe **jamais** l'implémentation d'un autre — il déclare une
   interface locale, le DI injecte ;
2. `platform/` n'accueille que de l'infrastructure, jamais une règle métier ;
3. `postgres.go` ne contient que du SQL — un algorithme qui tourne sans base
   remonte dans `service.go`.

Les migrations restent numérotées globalement : une base, une séquence. Le
découpage par capacité ne s'applique pas au schéma.

### Plan par phases

| Phase | Contenu | Sortie vérifiable | Risque |
|---|---|---|---|
| **0** | Option A en entier (§9) : remonter capacité et créneaux dans le domaine, une seule source pour les durées, clé d'idempotence hors de la vue | suite verte, règles de capacité testées **sans base** | faible |
| **1** | `platform/` : `postgres` (Open, pool, migrations), `httpserver`, `voice`, `config`, `sessions`, `logging` | build vert, aucun symbole renommé | faible |
| **2** | `tenant/` — le plus transverse, il conditionne tous les autres | contexte tenant et réglages inchangés de bout en bout | moyen |
| **3** | `identity/` — le plus isolé, 2 consommateurs | connexion réelle en navigateur | faible |
| **4** | `conversation/` (historique, consommation, webhook post-appel) | webhook signé + pages historique et consommation | moyen |
| **5** | `customer/`, `vehicle/`, `followup/` | fiche client réelle, panneau « à traiter » | faible |
| **6** | `appointment/` — le plus gros, deux entrées, **en dernier** | réservation comptoir **et** vocale, planning, statuts | élevé |
| **7** | `dashboard/`, `site/`, puis `app/` : `app.go` + `routes.go` remplacent `di` et `httpserver/handler.go` | inventaire des routes vert, smoke complet | moyen |

Une phase = un commit, suite complète verte et smoke sur l'application réelle
avant la suivante. Toute phase qui casse plus de trois fichiers de test hors
imports est le signe qu'elle est trop grosse : la couper.

### Ce que je recommande

**Faire la phase 0 maintenant, décider de la suite après.** Elle corrige les
seuls défauts que cet audit a pu prouver — des règles métier coincées dans
l'adaptateur SQL et une constante écrite quatre fois — sans déplacer un seul
package. Les phases 1 à 7 sont une amélioration de navigation : réelle, mais qui
ne change aucun comportement, alors que le produit n'a **encore jamais reçu un
seul appel téléphonique réel**.
