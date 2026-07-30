# WORKBOARD — deux agents

## Règles
- CLAIM avant code.
- Un seul owner par feature.
- Owned paths exclusifs pendant `IN_PROGRESS`.
- Les zones SERIAL exigent un handoff.
- Une feature doit être petite, testable et mergeable.
- **Une ligne de Journal après chaque commit, décision ou blocage.** Court, daté,
  signé. Un handoff long ne sert qu'à demander une review, pas à tenir informé.
- **Une demande à l'autre agent = une ligne dans Mini-tâches**, à laquelle on
  répond en éditant la ligne. Pas de question enterrée dans un bloc de 80 lignes.
- Avant de committer un fichier partagé (`handler.go`, `di.go`, `WORKBOARD.md`),
  relire `git diff` de ce fichier : si le diff de l'autre agent y est, le dire
  dans le message de commit au lieu de le committer en silence.

## Status
`READY` → `CLAIMED` → `IN_PROGRESS` → `REVIEW` → `MERGED`

## Journal — à lire en premier, à écrire en dernier

Une ligne par commit, décision ou blocage. La plus récente en bas. C'est ce
qu'un agent relancé à froid lit avant tout le reste. Format :
`AAAA-MM-JJ · Agent X · fait / décidé / bloqué : une phrase`.

- 2026-07-30 · Agent A · F00 mergé par le fondateur (PR #1, `229598e`).
- 2026-07-30 · Agent B · F06 CSS + tokens mergé ; templ `v0.3.1020` ajouté avec accord du fondateur.
- 2026-07-30 · Agent B · F04 dashboard en REVIEW (`GET /app`, fragment, 11 tests).
- 2026-07-30 · Agent A · F01, F02A, F03 en REVIEW ; contrats gelés avant code.
- 2026-07-30 · Agent A · F05 outils voix créneaux + réservation en REVIEW.
- 2026-07-30 · Agent B · fixture dashboard supprimée (`00f0b2c`) : le vrai provider est câblé.
- 2026-07-30 · Agent B · F07 site public + SEO en REVIEW (10 pages, robots, sitemap).
- 2026-07-30 · Agent B · bug corrigé : `robots.txt` laissait `/app` crawlable (`2b009f8`).
- 2026-07-30 · Agent A · F08 demande vocale de rappel/devis CLAIM, contrat gelé.
- 2026-07-30 · Agent B · F02B planning UI en REVIEW (`9e6edfe`) ; zone DI root libérée.
- 2026-07-30 · Agent A · F08 câblé dans le DI root et le routeur après release.
- 2026-07-30 · Agent B · décidé : la clé d'idempotence des formulaires planning dérive de `Appointment.UpdatedAt`, pas du `start`. Keyer sur le start rejouait une clé déjà dépensée après un aller-retour d'horaire et bloquait le déplacement suivant en 409. Amendement additif au contrat F04 (`views.Appointment.UpdatedAt`). Pas de mini-tâche : le correctif était côté B.
- 2026-07-30 · Agent A · F09 authentification staff CLAIM : contrat HTTP à geler avant code, sessions PostgreSQL et protection uniforme de `/app`; le diff local Agent B dans `planning_test.go` reste hors ownership.
- 2026-07-30 · Agent A · F09 en REVIEW : review indépendante PASS après correction SQL PostgreSQL réelle et ajout du budget anti-saturation CPU ; suite `-race` verte sur PostgreSQL 18.4.
- 2026-07-30 · Agent A · F10 CLAIM : corriger uniquement le mapping timezone du provider dashboard demandé dans MT-03, sans toucher aux vues Agent B.
- 2026-07-30 · Agent A · F10 en REVIEW : UTC converti explicitement vers le fuseau tenant, instant préservé ; review indépendante PASS et tests `-race` verts.
- 2026-07-30 · Agent A · F11 CLAIM : accepter MT-01 avec redirects d'erreur planning codés et contractés, sans rendre ni modifier les vues Agent B.
- 2026-07-30 · Agent A · F11 en REVIEW : les trois mutations redirigent vers quatre codes fermés sans fuite ni open redirect ; review indépendante PASS.
- 2026-07-30 · Agent A · F12 CLAIM : accepter MT-06 côté middleware après vérification officielle de `HX-Redirect`; aucune vue/login UI Agent B touchée.
- 2026-07-30 · Agent A · F12 en REVIEW : navigation HTML, HTMX et clients non-HTML distingués sans open redirect ; contrats amendés et review indépendante PASS.
- 2026-07-30 · Agent A · F14 CLAIM : persistance post-appel ElevenLabs et metering tenant/conversation ; F13 est laissé au login déjà nommé ainsi par Agent B, migration 00006 et DI réservées jusqu'à REVIEW.

- 2026-07-30 · Agent B · passe a11y mesurée dans Chrome sur `/app`, `/app/planning`, `/`, `/tarifs` : ordre de focus, noms accessibles, plan de titres, anneau de focus. Trois défauts corrigés : noms de contrôles dupliqués entre lignes de RDV, focus perdu après un swap htmx (htmx 2.0.10 ne restaure que par `id`, vérifié dans le fichier livré), accueil sans `aria-current`. 7 tests de non-régression ajoutés. **Reste manuel, non fait : un vrai lecteur d'écran (VoiceOver/NVDA) et une vraie touche Tab** — non pilotables depuis l'agent.
- 2026-07-30 · Agent B · constaté après F09 : `/app*` est protégé mais il n'existe aucune page de connexion, et le middleware répond en `text/plain`. L'app n'est pas utilisable dans un navigateur. Demande MT-06 ; je prends `GET /login` que le contrat F09 me laisse.
- 2026-07-30 · Agent B · **correctif de traçabilité** : le commit `f3ad1e4` porte un message qui décrit MT-06 alors que son contenu est l'acceptation de MT-01 et la ligne F11 écrites par Agent A (mon script d'édition avait échoué avant d'écrire, le commit est parti quand même). Aucun code touché. Le contenu réel de MT-06 arrive dans le commit suivant.
- 2026-07-30 · Agent B · F13 page de connexion livrée : `GET /login` (formulaire natif vers ton `POST /auth/login`, `autocomplete` username/current-password, messages d'échec volontairement vagues, `next` local seulement), lien Connexion dans le shell public, formulaire de déconnexion en POST dans le shell app, `Disallow: /login`. MT-07 posté pour les erreurs de login rendables et le `next`.
- 2026-07-30 · Agent B · boucle MT-01 fermée : le planning rend les quatre codes d'erreur de mutation de ton F11 en `role="alert"` rouge, distinct des notices jaunes, l'erreur survit à une journée illisible ou à une base en panne, un code inconnu retombe sur `unavailable` comme le contrat l'exige, et le code brut n'atteint jamais la page. 8 tests de plus.
- 2026-07-30 · Agent A · F14 en REVIEW : webhook HMAC vérifié contre SDK officiel, historique/metering atomique tenant-scopé ; review indépendante PASS et PostgreSQL 18.4 `-race` vert, zones migration/DI/Compose libérées.
- 2026-07-30 · Agent A · F16 CLAIM : accepter MT-07 côté handler auth, contrat F09 amendé avant code ; F15 laissé à l'historique d'appels UI Agent B, aucune vue/CSS touchée.
- 2026-07-30 · Agent B · MT-10 posté (renuméroté après collision avec MT-08 existant ; fragment `#planning-alert` sur ta redirection F11 pour que le focus rende l'alerte annonçable sans JS) ; ma moitié est déjà en place et inerte sans le fragment.
- 2026-07-30 · Agent B · F15 livré : contrat consommateur gelé dans `docs/contracts/F15-call-history.md`, puis `GET /app/calls` et `GET /app/calls/{id}` (liste du jour, résumé étiqueté « non vérifié », transcription par tours), 11 tests, lien Appels dans la nav. **Volontairement non câblé** : il manque l'adaptateur sur ta table `conversations` (MT-09), et je refuse de brancher une route de production sur une fixture — c'est ce qu'on a supprimé avec `dashboard_fixture.go`. Dès que l'adaptateur existe : une ligne de DI et deux `Register`.
- 2026-07-30 · Agent A · F16 en REVIEW : erreurs login HTML/HTMX rendables, clients API inchangés et `next` strictement local ; review indépendante PASS, zones auth libérées.
- 2026-07-30 · Agent A · F17 CLAIM : accepter MT-10 par fragment de focus fixe sur les seuls redirects d'échec planning ; aucune vue Agent B touchée.
- 2026-07-30 · Agent A · F17 en REVIEW : fragment fixe ajouté aux échecs seulement, codes/succès inchangés ; review indépendante PASS et tests `-race` verts.
- 2026-07-30 · Agent B · review croisée F14 : **PASS**, deux findings faibles (horodatage non borné dans le futur, en-tête de signature intolérant à un espace) écrits plus bas. Signature, conflit, idempotence, arrondi half-up et isolation vérifiés en exécutant contre PostgreSQL 18.4 réel. MT-10 posté : les données existent, les deux panneaux vides du dashboard n'attendent plus qu'un adaptateur.
- 2026-07-30 · Agent A · F18 CLAIM : accepter MT-09 pour le read model tenant-scopé F15 et remplir le panneau Appels F04 ; contrat F15 inchangé, routes/UI Agent B intactes, DI root réservée jusqu'à REVIEW. La nouvelle demande dashboard du commit Agent B est renumérotée MT-12 car MT-10 et MT-11 existaient déjà.
- 2026-07-30 · Agent B · MT-07 et MT-08 vérifiés bout en bout contre l'app réelle et passés `done` : un mot de passe faux renvoie maintenant le garage sur `/login` avec le message et le `next` conservés, et l'alerte de mutation reçoit le focus via `#planning-alert`. Finding au passage → MT-11 : `next` peut pointer sur une route POST-only.
- 2026-07-30 · Agent A · F18 en REVIEW : historique F15 et panneau Appels câblés sur le read model tenant-scopé ; review indépendante PASS après canonicalisation UUID, PostgreSQL 18.4 et suite complète `-race` verts. DI root libérée.
- 2026-07-30 · Agent B · **reprise du rôle backend** : Agent A a quitté la session. Sa dernière livraison (`fdce706`, MT-09/MT-10 partiels) est reviewée et câblée. Les mini-tâches restantes deviennent ma file de travail.
- 2026-07-30 · Agent B · MT-11 fermée (le `next` n'est porté que pour un GET) et les deux findings de ma review F14 corrigés (horodatage borné dans le futur, en-tête de signature tolérant aux espaces), chacun avec son test.
- 2026-07-30 · Agent B · panneau « À traiter » enfin rempli : modèle de lecture des demandes de rappel (plus ancienne d'abord, nom client résolu par LEFT JOIN dans le même aller-retour, index partiel de F08 utilisé). Le service refuse une ligne d'un autre tenant, non `pending`, ou sans date.
- 2026-07-30 · Agent B · identité de l'appelant dans l'historique : **le payload post-call d'ElevenLabs ne documente aucun numéro** (vérifié le 2026-07-30 sur la doc officielle), donc l'identité vient de ce que nos propres outils ont enregistré (F08 : `conversation_id` + téléphone + client). Une requête groupée par page, jamais une par ligne ; un échec coûte un nom, pas la journée.
- 2026-07-30 · Agent B · **trou produit trouvé et bouché** : `auth.Provision` existait mais rien ne l'appelait — un déploiement neuf n'avait aucun compte, donc personne ne pouvait jamais se connecter. `cmd/provisionstaff` livré (mot de passe sur stdin).
- 2026-07-30 · Agent B · e2e complet vérifié contre l'app réelle : outil vocal rappel → webhook post-appel → compte créé → connexion navigateur → le dashboard affiche l'appel et la demande, l'historique affiche le numéro, la durée et le résumé. Badge `success` traduit en « Abouti » (l'anglais brut du fournisseur passait à l'écran).
- 2026-07-30 · Agent B · **découpage par feature livré** : `internal/features/{dashboard,planning,calls,site,identity,voicetools,postcall}`, chacune propriétaire de son handler, ses routes, ses gabarits et ses tests. Le routeur racine ne nomme plus aucune URL de l'app. Partagés : `internal/web/views` (kit UI), `internal/web/page` (Render/RequestedDay/Origin), `internal/adapters/voice` (credential + préambule). Suite a11y transverse dans son propre package `internal/web/a11y`. Doctrine écrite dans AGENTS.md. Dossiers morts supprimés (`adapters/handlers`, `stores/sqlite`, `assets/templates`, `assets/src/ts`, `assets/dist`, `core/ports`).
- 2026-07-30 · Agent B · **point 2 livré** : un appelant inconnu peut enfin obtenir un rendez-vous. `POST /voice/tools/customer-record` crée le client et rattache la plaque, puis `appointment-book` prend le relais avec le `customer_id` renvoyé. Vérifié bout en bout sur l'app réelle : inconnu → enregistré → créneaux → RDV confirmé → visible au planning avec la plaque. Amendement de mon propre contrat avant code : `first_name` était marqué requis à tort — un appelant sans nom doit rester réservable, le numéro est l'identité.
- 2026-07-30 · Agent B · **point 4 livré** : la page Consommation existe. Quota mensuel par tenant en base (défaut 750 min, l'offre d'entrée du PRD), minutes arrondies à la minute supérieure par appel, alertes 70/85/100 %. La page tarifs promettait ces alertes depuis le premier jour ; elles sont vraies maintenant. Élément natif `<meter>` plutôt qu'une barre en div : il porte la valeur, le maximum et les seuils aux technologies d'assistance sans une seule ligne d'ARIA.
- 2026-07-30 · Agent B · bug d'infrastructure trouvé en chemin : deux packages de test qui ouvrent la même base migrent **en même temps**, l'un crée un index sur lequel l'autre échoue, et la version reste à moitié appliquée. Corrigé avec un verrou consultatif de session (`goose.WithSessionLocker`, vérifié dans `go doc`), pas avec un `IF NOT EXISTS` qui aurait masqué la course. Vaut aussi pour un déploiement à deux instances qui démarrent ensemble.
- 2026-07-30 · Agent B · **point 5b livré** : le statut d'intervention se pilote depuis le planning (Confirmer, Démarrer, Terminer, Client absent). Pas de nouvelle entité : le rendez-vous porte l'état, avec la table de transitions que F02A avait gelée sans jamais l'exposer — `ErrInvalidTransition` était déclarée et jamais renvoyée. Le contrôle est dans l'UPDATE, pas dans un read-puis-write : deux personnes au comptoir ne peuvent pas croire toutes les deux avoir gagné.
- 2026-07-30 · Agent B · **point 5a livré** : les fiches client existent enfin. Recherche par nom, numéro **ou plaque** — les trois choses qu'un appelant donne — puis fiche avec véhicules et passages. Lecture seule assumée : c'est l'assistant qui écrit ces fiches pendant les appels (F19), un formulaire d'édition au comptoir est une décision séparée.
- 2026-07-30 · Agent B · **point 3, la moitié qui nous revient** : page Réglages. Le numéro de transfert et le quota mensuel étaient l'un une colonne inexistante, l'autre une valeur que personne ne pouvait changer — le même piège que les horaires. Le déclenchement du transfert reste chez ElevenLabs (vérifié dans leur doc : initié par l'agent, aucun déclenchement externe documenté), et la page le dit au lieu de le laisser croire.
- 2026-07-30 · Agent B · `NormalizePhone` déplacée dans `core/domain` : trois domaines en ont besoin (client, rappel, numéro de transfert de l'atelier) et `tenant` ne peut pas importer `customer` sans cycle. `customer.NormalizePhone` reste le nom gelé par F01 et délègue.
- 2026-07-30 · Agent B · identité légale renseignée depuis les projets voisins, sur confirmation du fondateur que c'est la même entité : entreprise individuelle Alexandre Désir, SIRET 105 156 814 00011 (source `verbobum/docs/REGISTRE-DONNEES-PERSONNELLES.md`), contact `contact@bureau-mq.com` (source `bureau-mq/internal/agent/profile.md`). Restent `[À VALIDER]` parce qu'ils n'existent dans **aucun** projet : hébergeur (dénomination, adresse, téléphone), adresse de l'établissement, numéro de téléphone. Pas de palette à hériter : bureau-mq et verbobum utilisent des thèmes daisyUI du catalogue (`bumblebee`, `business`), le garage a son propre système de tokens.
- 2026-07-30 · Agent B · sauvegardes livrées (PRD §12) : dump quotidien vérifié puis tourné, script de restauration qui refuse de viser la production. Compromis écrit noir sur blanc dans `docs/BACKUPS.md` : un dump logique n'est pas du PITR, **on peut perdre une journée**, et les trois conditions qui doivent déclencher le passage au WAL sont nommées. `BACKUP_DIR` doit pointer hors du VPS ; tant que ce stockage n'est pas choisi, l'exigence du PRD n'est pas satisfaite et la doc le dit.
- 2026-07-30 · Agent B · `features/customers` renommé `features/customerfiles` : `core/customer` et `features/customers` se ressemblaient trop dans le DI. Les deux couches restent séparées à dessein — chaque domaine du coeur sert 3 à 8 features (mesuré), les fusionner dupliquerait les règles ou créerait des imports feature→feature.

## Mini-tâches et demandes de contrat

Une ligne par demande. On répond **en éditant la ligne** (statut + une phrase),
pas en ajoutant un bloc en fin de fichier. `open` → `accepted` / `refused` /
`done`.

| ID | Demandeur → cible | Sujet | Statut |
|---|---|---|---|
| MT-01 | Agent B → Agent A | Les trois POST F02A renvoient leurs erreurs en `text/plain` via `http.Error`. Après un 409 le garage lit « appointment conflict » en police brute, alors que le contrat F02A dit « F02B renders human-readable HTML from these outcomes ». Deux options : (a) rediriger en 303 vers `/app/planning?day=...&error=<code>` et je rends le message, (b) tu m'appelles pour rendre la page d'erreur. Je préfère (a) : ça reste ton handler, aucun couplage de vue chez toi, et un rechargement reste correct. | done — F11 livre `303 /app/planning?error=<code>`; jour omis quand l'écriture échouée ne permet pas de date fiable |
| MT-02 | Agent B → Agent A | `internal/adapters/httpserver/handler.go` et `internal/di/di.go` sont devenus notre point de collision : trois fois aujourd'hui nos deux diffs ont atterri dans le même hunk. Proposition : chaque groupe de handlers expose `Register(mux *http.ServeMux)` (comme `handlers.Site` le fait déjà) et `httpserver.New` n'appelle plus que ces `Register`. Chacun édite alors son propre fichier. Ton appel, c'est ton fichier contracté. | open |
| MT-03 | Agent B → Agent A | Vérification demandée, pas un changement : `internal/adapters/handlers/appointment_today_provider.go` passe `entry.Start` tel quel à la vue F04. Si pgx rend `timestamptz` en UTC, le dashboard affiche des heures UTC au lieu de l'heure atelier. Mon adaptateur planning convertit explicitement via `.In(day.Date.Location())`. Je n'ai pas pu l'observer en vrai (`/app` est dégradé sans tenant). | done — F10 convertit Start/End via le fuseau de `planningDay.Date`, test UTC→Martinique |
| MT-04 | Agent A → contrats F02A/F04 | F09 rend l'authentification navigateur effective : sans session valide, toutes les routes `/app` répondent désormais `401`, avant tout handler. Cela remplace explicitement le `200` dégradé F04 sans tenant et lève l'interdiction de déploiement public des mutations F02A une fois la protection CSRF active. | accepted — mini-tâche F09, amendements documentés avant implémentation |
| MT-05 | Review F09 → Agent A | Les emails inconnus exécutent volontairement PBKDF2 mais contournent le verrouillage par compte : borner le endpoint à 30 dérivations/minute et 2 simultanées par processus, avec `429`/`Retry-After`, avant de considérer F09 déployable sur le petit VPS. | done — contrat, limiteur et tests concurrence/budget/reset livrés |
| MT-06 | Agent B → Agent A | `requireStaffSession` répond `401 text/plain` « authentication required ». Dans un navigateur c'est une page nue en police brute, sans issue : il n'existe aucune page de connexion. Le contrat F09 me laisse explicitement `GET /login`, je le construis. La moitié manquante est chez toi : que le middleware distingue les deux publics — navigation HTML (`Sec-Fetch-Mode: navigate` ou `Accept: text/html`) → `303` vers `/login?next=<chemin>` ; requête htmx (`HX-Request: true`) → `401` + en-tête `HX-Redirect: /login` (mécanisme présent dans le htmx 2.0.10 que je livre, vérifié dans le fichier) ; le reste inchangé. Trois lignes dans ton middleware, aucun changement sur `POST /auth/login`. | done — F12 livre la matrice contractée; `next` vient uniquement de l'URL locale `/app` et est encodé |
| MT-08 | Agent A → Agent B | F14 doit monter une route dans `internal/adapters/httpserver/handler.go`, où ton GET `/login` avait un diff local non commité. Agent A a attendu sans prendre le diff. | done — Agent B a commité F13 sous `e6006df` et libéré le routeur |
| MT-07 | Agent B → Agent A | `POST /auth/login` renvoie ses échecs en `text/plain` (`401`/`422`/`415`/`429`). Dans un navigateur, un mot de passe faux fait perdre le formulaire et affiche une page nue — le même problème que MT-01, sur le chemin le plus emprunté du produit. Demande, même matrice que ton F12 : navigation HTML → `303` vers `/login?error=<code>` avec le jeu fermé `invalid|rate_limited|unavailable|forbidden` (les `401`, `422` et `415` retombent tous sur `invalid`, pour ne rien révéler) ; `HX-Request` → `401` + `HX-Redirect` ; autre client → inchangé ; `Retry-After` conservé sur le `429`. Ma page rend déjà ces quatre codes. Second point : honorer un champ de formulaire optionnel `next`, validé côté serveur (chemin local `/app` uniquement), comme cible du `303` après succès à la place de `/app` — mon formulaire l'envoie déjà, ton handler l'ignore aujourd'hui. | done — F16 livre la matrice contractée; API inchangée, `next` canonique `/app` seulement, review PASS |
| MT-10 | Agent B → Agent A | Une `role="alert"` déjà présente dans le document au chargement n'est pas annoncée de façon fiable par les lecteurs d'écran : la live region doit exister *avant* le changement. Après ta redirection F11, l'opérateur aveugle peut donc ne jamais entendre que son action a échoué. Le seul correctif vérifiable sans JavaScript est de laisser le navigateur poser le focus : ajoute le fragment à ta redirection, `303 /app/planning?error=<code>#planning-alert`. Ma moitié est déjà en place (`id="planning-alert" tabindex="-1"`, inerte sans le fragment, hors ordre de tabulation). Un fragment n'est jamais envoyé au serveur, donc aucun risque d'injection et aucun changement de code fermé. | done — F17 livre le fragment fixe sur les seuls échecs; review PASS |
| MT-09 | Agent B → Agent A | F14 persiste les conversations mais n'expose aucun modèle de lecture : je ne peux pas afficher l'historique et les résumés d'appel. Contrat consommateur gelé de mon côté dans `docs/contracts/F15-call-history.md` (routes `GET /app/calls` et `GET /app/calls/{id}`, seam Go et DTO de présentation, `tenant_id` uniquement depuis le contexte). Demande : un adaptateur qui satisfait ce seam au-dessus de ta table `conversations`, comme tu l'as fait pour F04. Bonus au même endroit : le panneau « Appels » du dashboard est vide depuis le début parce que ton adaptateur F04 renvoie `Calls: []` — le même modèle de lecture le remplit. Mes vues, mon handler et mes tests sont prêts et livrés non câblés : il ne manque que l'adaptateur et une ligne de DI. | done — F18 read model + adaptateur + routes protégées + panneau Appels ; contrat F15 inchangé, review PASS |
| MT-12 | Agent B → Agent A | Le dashboard a deux panneaux vides depuis le premier jour parce que ton adaptateur F04 renvoie `Calls: []` et `Tasks: []`. Les données existent maintenant : `conversations` (F14) et `follow_up_requests` (F08). Aucun travail d'UI n'est nécessaire, les DTO `views.Call` et `views.Task` sont déjà gelés et rendus. Demande : remplir ces deux tranches dans `appointment_today_provider.go` (ou un adaptateur voisin) depuis les modèles de lecture. C'est le gain produit le moins cher qui reste : la page du jour devient réellement la page du jour. | accepted — appels done dans F18 ; tâches follow-up restent ouvertes pour une feature séparée |
| MT-11 | Agent B → Agent A | Finding trouvé en vérifiant MT-07 : ta dérivation de `next` (F12) accepte n'importe quelle route `/app`, y compris celles qui n'existent qu'en POST. Un `POST /app/appointments/<id>/cancel` sans session redirige vers `/login?next=%2Fapp%2Fappointments%2F<id>%2Fcancel` — vérifié. Après reconnexion, suivre ce `next` ferait un GET sur une route POST-only : le garage atterrit sur une erreur au lieu du planning. Suggestion : ne porter `next` que pour une requête `GET` (la méthode est à portée de main dans le middleware), sinon l'omettre et laisser le défaut `/app`. Côté page je n'ai rien à changer : je valide déjà le préfixe, pas la méthode, que je ne connais pas. | open |

> **Chemins d'avant le découpage.** Les lignes closes ci-dessous citent la
> disposition d'origine ; elles ne sont pas réécrites, le registre reste ce qui
> s'est passé. La correspondance :
> `internal/adapters/handlers/dashboard*` → `internal/features/dashboard/**` ·
> `…/planning*`, `…/appointment*`, `…/opening*` → `internal/features/planning/**` ·
> `…/calls*`, `…/call_history*` → `internal/features/calls/**` ·
> `…/site*` → `internal/features/site/**` ·
> `…/login*`, `…/auth*` → `internal/features/identity/**` ·
> `internal/adapters/voice/{customer_lookup,appointment_booking,followup}*` → `internal/features/voicetools/**` ·
> `internal/adapters/voice/post_call*` → `internal/features/postcall/**` ·
> les vues de page → leur feature, le kit partagé restant dans `internal/web/views`.

| ID | Feature | Owner | Depends on | Owned paths | Contract/API frozen | Tests / acceptance | Status |
|---|---|---|---|---|---|---|---|
| F00 | Socle PostgreSQL 18 + pgx + Goose | Agent A | - | `go.mod`, `go.sum`, `Dockerfile`, `compose.yml`, `.github/workflows/ci.yml`, `internal/config/**`, `internal/di/**`, `internal/adapters/stores/postgres/**`, suppression de `internal/adapters/stores/sqlite/**`, `docs/POSTGRESQL.md`, `README.md` | `postgres.Store` satisfait `ports.ReadinessStore`; migrations SQL embarquées | démarrage, migrations idempotentes, readiness, tests PostgreSQL | MERGED |
| F01 | Tenant + Customer + Vehicle | Agent A | F00 | `docs/contracts/F01-customer-vehicle.md`, `docs/DATABASE.md`, `internal/core/tenant/**`, `internal/core/customer/**`, `internal/core/vehicle/**`, `internal/adapters/stores/postgres/tenant*.go`, `internal/adapters/stores/postgres/customer*.go`, `internal/adapters/stores/postgres/vehicle*.go`, `internal/adapters/stores/postgres/migrations/00002_tenant_customer_vehicle.sql`, `cmd/main.go` (import stdlib `time/tzdata` uniquement) | `docs/contracts/F01-customer-vehicle.md` (frozen 2026-07-30); `tenant_id` uniquement depuis contexte serveur | create/find by phone, tenant isolation | REVIEW |
| F02A | Mini-planning atelier — backend | Agent A | F00,F01 | `docs/contracts/F02A-planning.md`, `docs/SCHEDULING.md`, `internal/core/appointment/**`, `internal/adapters/stores/postgres/appointment*.go`, `internal/adapters/stores/postgres/migrations/00003_appointment.sql`, `internal/adapters/handlers/appointment*.go`, `internal/adapters/httpserver/handler.go` (routing contracté uniquement), `internal/di/**` (wiring uniquement), suppression de `internal/adapters/handlers/dashboard_fixture.go` après adaptateur réel | `docs/contracts/F02A-planning.md` (frozen 2026-07-30); `tenant_id` uniquement depuis contexte; recheck atomique; idempotence | disponibilité + créer/déplacer/annuler + tenant isolation + dashboard réel | REVIEW |
| F02B | Mini-planning atelier — UI | Agent B | F02A | `internal/features/planning/**`, `internal/web/views/layout.templ` (un lien de nav), `assets/src/css/app.css` (composants planning), `internal/adapters/httpserver/handler.go` (deux routes GET), `internal/di/**` (wiring uniquement) | consomme `docs/contracts/F02A-planning.md` (gelé) : deux GET à moi, les trois POST à Agent A ; aucun `tenant_id` en route/form/DTO | 16 tests ; `?day=` lu dans le timezone tenant, heures converties, créneaux par durée de RDV, clé d'idempotence déterministe, dégradation partielle ; boot réel PostgreSQL 18.4 ; screenshots 1280 + 380 px réels, clair/sombre, replié/déplié | REVIEW |
| F03 | Voice lookup customer tool | Agent A | F01 | `docs/contracts/F03-voice-customer-lookup.md`, `docs/ELEVENLABS.md`, `internal/features/voicetools/**`, `internal/features/postcall/**`, `internal/adapters/voice/**` (credential partagé), `internal/config/**` (variable credentials uniquement), `compose.yml` (une variable app), `internal/adapters/httpserver/handler.go` (une route), `internal/di/**` (wiring uniquement) | `docs/contracts/F03-voice-customer-lookup.md` (frozen 2026-07-30); secret → tenant context, jamais tenant_id LLM | known + unknown phone + auth/isolation + erreurs bornées | REVIEW |
| F04 | Dashboard Today | Agent B | F02A | `internal/features/dashboard/**` | `docs/contracts/F04-dashboard-today.md` (frozen 2026-07-30) | calls/RDV/tasks render | REVIEW — page servie sur `GET /app`, fragment `GET /app/today`, 11 tests, vérifiée en navigateur à 380 et 1280 px |
| F05 | Voice find slots + book appointment | Agent A | F02A,F03 | `docs/contracts/F05-voice-book-appointment.md`, `docs/ELEVENLABS.md` (ajout F05 uniquement), `internal/adapters/voice/appointment_booking*.go`, `internal/adapters/httpserver/handler.go` (deux routes uniquement), `internal/di/**` (wiring uniquement) | `docs/contracts/F05-voice-book-appointment.md` (frozen 2026-07-30); interfaces `SchedulingProvider` inchangées | disponibilité persistée + confirmation après commit + auth/isolation + idempotence déterministe | REVIEW |
| F06 | CSS tokens + base components | Agent B | - | `assets/src/css/**` | existing token names in `assets/src/css/tokens.css` | responsive/a11y smoke — DONE: light+dark at 360/500/700/1280, no overflow, 3 defects fixed | MERGED |
| F07 | Site public + SEO (PRD §11) | Agent B | - | `internal/features/site/**`, `assets/src/css/site.css`, `assets/src/css/app.css` (un `@import`), `internal/adapters/httpserver/handler.go` (une ligne de montage) | routes `/`, `/fonctionnalites`, `/tarifs`, `/garages`, `/demo`, `/contact`, `/mentions-legales`, `/confidentialite`, `/cgv`, `/cgu`, `/robots.txt`, `/sitemap.xml` — aucun `tenant_id`, aucun état serveur | 12 tests ; SEO head par page, sitemap trié, footer sans 404, CTA sans self-link ; smoke sur PostgreSQL 18.4 réel ; screenshots 1280 + 380 px réels, clair et sombre | REVIEW |
| F08 | Demande vocale de rappel/devis | Agent A | F01,F03 | `docs/contracts/F08-follow-up-request.md`, `docs/DATABASE.md` (ajout F08), `docs/ELEVENLABS.md` (ajout F08), `internal/core/followup/**`, `internal/adapters/stores/postgres/followup*.go`, `internal/adapters/stores/postgres/migrations/00004_follow_up_request.sql`, `internal/adapters/voice/followup*.go`, `internal/adapters/httpserver/handler.go` (une route), `internal/di/**` (wiring uniquement) | `docs/contracts/F08-follow-up-request.md` (frozen 2026-07-30); tenant depuis Bearer, liaison client par téléphone côté serveur | connu/inconnu + tenant isolation + rejeu identique + conflit + erreurs bornées | REVIEW |
| F09 | Authentification staff et sessions navigateur | Agent A | F01,F02A,F04 | `docs/contracts/F09-authentication.md`, `docs/SECURITY.md`, `docs/DATABASE.md` (ajout F09), amendements auth uniquement aux contrats F02A/F04, `internal/core/auth/**`, `internal/adapters/stores/postgres/auth*.go`, `internal/adapters/stores/postgres/migrations/00005_authentication.sql`, `internal/adapters/handlers/auth*.go`, `internal/adapters/httpserver/**` (middleware/routing auth uniquement), `internal/di/**` (wiring uniquement) | `docs/contracts/F09-authentication.md` (gelé 2026-07-30); email résout le tenant côté serveur; cookie opaque, secret non stocké; `/app` protégé uniformément | mot de passe/session + login/logout + CSRF + expiration/révocation + isolation tenant + PostgreSQL 18 réel | REVIEW |
| F10 | Dashboard : heures dans le fuseau tenant | Agent A | F02A,F04 | `internal/adapters/handlers/appointment_today_provider.go`, test ciblé dans `appointment_test.go`, `WORKBOARD.md` | seam F04 inchangé; `planningDay.Date.Location()` est la source du fuseau | timestamps UTC du store rendus en heure Martinique, instant préservé | REVIEW |
| F11 | Planning : erreurs de mutation rendables | Agent A | F02A,F02B,F09 | amendement `docs/contracts/F02A-planning.md`, `internal/adapters/handlers/appointment.go`, tests mutation, `WORKBOARD.md` | erreur fermée `invalid|not_found|conflict|unavailable`; aucun message/return URL libre | chaque erreur post-auth redirige 303 sans fuite; 401 middleware inchangé | REVIEW |
| F12 | Session expirée : retour connexion | Agent A | F09,MT-06 | amendements `docs/contracts/F09-authentication.md`, `F02A-planning.md`, `F04-dashboard-today.md`, `internal/adapters/httpserver/middleware.go`, tests middleware, `WORKBOARD.md` | navigation HTML→303 local; HTMX→401+HX-Redirect; autres→401; 503 inchangé | next encodé sans open redirect + matrice missing/invalid/unavailable | REVIEW |
| F14 | Post-appel : historique, résumé et metering | Agent A | F01,F03 | `docs/contracts/F14-post-call.md`, ajouts F14 à `docs/ELEVENLABS.md` et `docs/DATABASE.md`, `internal/core/conversation/**`, `internal/adapters/stores/postgres/conversation*.go`, migration `00006_conversation.sql`, `internal/adapters/voice/post_call*.go`, `internal/config/**` et `compose.yml` (variables F14 uniquement), `internal/adapters/httpserver/handler.go` et `internal/di/**` (wiring F14 uniquement), `WORKBOARD.md` | `docs/contracts/F14-post-call.md` gelé le 2026-07-30 : `POST /webhooks/elevenlabs/post-call`; signature sur corps brut; tenant par mapping serveur agent→tenant; événement idempotent | signature/timestamp/corps borné + doublon/conflit + tenant isolation + PostgreSQL 18 réel + metering durée/coût fiat | REVIEW |
| F13 | Page de connexion (front) | Agent B | F09,F12 | `internal/features/identity/**`, `internal/web/views/layout.templ` (formulaire de déconnexion), `internal/web/views/site_layout.templ` (lien Connexion), `assets/src/css/app.css`, `internal/adapters/handlers/site.go` (`Disallow: /login`), `internal/adapters/httpserver/handler.go` (une ligne) | consomme `POST /auth/login` gelé (F09) : `email`, `password`, form-urlencoded ; `GET /login` laissé au frontend par le contrat | 6 tests login + a11y étendue ; messages d'échec indistinguables, `next` local uniquement, `no-store` + `noindex` ; focus et labels mesurés dans Chrome ; screenshot 1280 relu | REVIEW |
| F16 | Login navigateur : erreurs rendables et retour local | Agent A | F09,F13,MT-07 | amendement `docs/contracts/F09-authentication.md`, `internal/adapters/handlers/auth.go`, `internal/adapters/handlers/auth_test.go`, `WORKBOARD.md` | navigation HTML→303 vers code fermé; HTMX→401+HX-Redirect; API inchangée; `next` local `/app` seulement | aucun open redirect/enumeration + matrice erreurs + Retry-After + succès next | REVIEW |
| F15 | Historique et résumés d'appel — UI | Agent B | F14,F09 | `docs/contracts/F15-call-history.md`, `internal/features/calls/**`, `internal/web/views/layout.templ` (un lien de nav), `assets/src/css/app.css` | `docs/contracts/F15-call-history.md` (gelé 2026-07-30 par Agent B, côté consommateur) : `GET /app/calls` + `GET /app/calls/{id}`, seam `CallHistoryReader`, DTO de présentation, `tenant_id` depuis le contexte | 11 tests ; timezone tenant, date illisible, lecture en panne ≠ journée vide, introuvable ≠ indisponible, rôle inconnu brut, résumé étiqueté non vérifié ; a11y étendue aux deux pages ; screenshots 1280 relus | REVIEW — backend F18 câblé, contrat/UI inchangés |
| F17 | Planning : focus de l'alerte après échec | Agent A | F11,F15,MT-10 | amendement `docs/contracts/F02A-planning.md`, `internal/adapters/handlers/appointment.go`, `internal/adapters/handlers/appointment_test.go`, `WORKBOARD.md` | échec seulement→`303 /app/planning?error=<code>#planning-alert`; codes inchangés | quatre codes + parse/form/service, succès sans fragment | REVIEW |
| F18 | Historique d'appels — read model backend | Agent A | F14,F15,F09,MT-09 | `internal/core/conversation/**` (lecture uniquement), `internal/adapters/stores/postgres/conversation*.go` (lecture uniquement), nouveaux `internal/adapters/handlers/call_history_provider*.go` et `today_provider*.go`, `internal/adapters/httpserver/handler.go` (Register F15 uniquement), `internal/di/di.go` (wiring uniquement), `WORKBOARD.md` | consomme `docs/contracts/F15-call-history.md` sans changement ; ID opaque, jour civil tenant, `tenant_id` depuis contexte uniquement | ordre récent, timezone, transcript, ID inconnu/étranger indistinguables, PostgreSQL 18 réel, routes sous session, panneau Appels réel | REVIEW — review indépendante PASS, PostgreSQL 18.4 `-race` vert |
| F16 | Reprise du rôle backend par Agent B | Agent B | - | tous les chemins backend | Agent A a quitté la session ; plus de partage d'ownership, le protocole de mini-tâches devient une file de travail | suite complète verte avec `TEST_DATABASE_DSN` | IN_PROGRESS |
| F17 | File de rappels au dashboard + identité de l'appelant | Agent B | F08,F14,F15 | `internal/core/followup/read*.go`, `internal/adapters/stores/postgres/followup_read*.go`, `internal/adapters/handlers/followup_provider.go`, `call_history_provider.go`, `internal/di/di.go`, `internal/web/views/today.go` | l'identité vient de nos propres outils (F08), jamais d'un champ fournisseur non documenté | isolation tenant + ordre + appelant connu/inconnu sur PostgreSQL réel ; e2e complet appel→dashboard | REVIEW |
| F18 | Commande de création du premier compte | Agent B | F09 | `cmd/provisionstaff/**` | mot de passe lu sur stdin, jamais en flag ; tenant depuis le contexte comme derrière une session | compte créé puis connexion navigateur réelle vérifiée | REVIEW |
| F19 | Outil voix : enregistrer l'appelant et sa plaque | Agent B | F01,F03 | `docs/contracts/F19-voice-customer-create.md`, `docs/ELEVENLABS.md` (ajout F19), `internal/features/voicetools/customer_record*.go`, `internal/adapters/httpserver/handler.go` (une route), `internal/di/**` | `docs/contracts/F19-voice-customer-create.md` (gelé 2026-07-30, amendé avant code : `first_name` optionnel) | 6 tests sur PostgreSQL réel : création, rejeu sans doublon, nom jamais écrasé, plaque d'un autre client en 409, entrées refusées, token exigé ; e2e appel inconnu → RDV au planning | REVIEW |
| F20 | Consommation : minutes, quota et alertes | Agent B | F14 | `internal/adapters/stores/postgres/migrations/00008_usage_quota.sql`, `internal/core/conversation/usage.go`, `internal/adapters/stores/postgres/conversation_usage.go`, `internal/features/usage/**`, `internal/web/views/layout.templ` (un lien), `assets/src/css/app.css` | `GET /app/usage?month=YYYY-MM`, quota par tenant en base, alertes 70/85/100 % (PRD §5) | 7 tests ; mois lu dans le fuseau tenant, arrondi minute supérieure, dépassement visible, dégradation sans fuite ; vérifié sur l'app réelle (624/750 min, 83 %) | REVIEW |
| F21 | Statut d'intervention depuis le comptoir | Agent B | F02A | amendement `docs/contracts/F02A-planning.md`, `internal/core/appointment/**`, `internal/adapters/stores/postgres/appointment.go`, `internal/features/planning/**` | `POST /app/appointments/{id}/status` ; table de transitions gelée, appliquée dans l'UPDATE | table de transitions testée exhaustivement, rejeu no-op, état terminal figé, isolation tenant indiscernable d'un refus ; vérifié sur l'app réelle (RDV passé « En cours ») | REVIEW |
| F22 | Fiches client et véhicule — UI | Agent B | F01,F19 | `internal/core/customer/read.go`, `internal/adapters/stores/postgres/customer_read.go`, `internal/features/customers/**`, `internal/web/views/layout.templ` (un lien) | `GET /app/customers?q=` et `GET /app/customers/{id}` ; recherche nom/numéro/plaque, lecture seule | 4 tests handler + recherche sur PostgreSQL réel (nom, numéro, plaque, isolation tenant, fiche d'un autre atelier introuvable) ; a11y étendue ; vérifié sur l'app réelle | REVIEW |
| F23 | Réglages atelier : transfert et quota | Agent B | F09,F20 | `internal/adapters/stores/postgres/migrations/00009_tenant_settings.sql`, `internal/core/tenant/**`, `internal/core/domain/phone.go`, `internal/adapters/stores/postgres/tenant.go`, `internal/features/settings/**` | `GET`/`POST /app/settings` ; numéro normalisé E.164, quota 1–100000 | tests domaine (normalisation, effacement, bornes) ; enregistré et relu sur l'app réelle, le quota se propage à la page Consommation | REVIEW |
| F24 | Sauvegardes PostgreSQL quotidiennes | Agent B | F00 | `ops/backup.sh`, `ops/restore.sh`, `docs/BACKUPS.md`, `compose.yml` (service sous profil) | dump logique cohérent, contrôle de taille + `pg_restore --list`, rotation après succès seulement ; restauration refusée hors base de test | dump et restauration vérifiés sur base réelle (133 ateliers, 104 clients, 60 appels retrouvés) ; **démarrage de l'app sur la base restaurée non vérifié** (démon Docker arrêté) | REVIEW |

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

## Coordination F05 unblock — Agent A, 2026-07-30

F05 dépend du backend F02A, pas des vues F02B. F02A et F03 sont poussés et en
`REVIEW`; leurs contrats gelés fournissent respectivement `SchedulingProvider`
et l'authentification voix tenant-scoped. Le blocage technique est donc levé.
F05 est CLAIM par Agent A et ne touche aucun chemin possédé par Agent B.

Le contrat HTTP F05 est gelé avant implémentation dans
`docs/contracts/F05-voice-book-appointment.md`. Les interfaces provider F02A
restent inchangées. F02B peut continuer indépendamment.

## Coordination F08 — Agent A, 2026-07-30

F08 est CLAIM sans prendre la zone DI détenue par Agent B pour F02B. Agent A
fige puis implémente le contrat, le domaine, la migration/store PostgreSQL et le
handler voix dans des chemins indépendants. Le montage de la route et le wiring
DI restent différés jusqu'au handoff explicite F02B ; Agent A ne modifiera pas
ces deux zones pendant l'ownership Agent B.

La demande ne reçoit ni `tenant_id`, ni `customer_id`, ni statut depuis le LLM.
Le Bearer F03 établit le tenant et le store rattache éventuellement le client
par téléphone normalisé dans ce même tenant. Aucun provider externe ni nouvelle
dépendance n'est introduit.

### Handoff résolu — Agent A et Agent B pour F08

Agent B a livré F02B dans `9e6edfe`, passé la feature en `REVIEW` et libéré la
DI dans `6ce3f79`. Agent A CLAIM maintenant la DI pour construire
`FollowUpTool`, ajouter un argument à `httpserver.New` et monter uniquement
`POST /voice/tools/follow-up-request`. Aucun fichier vue/CSS/handler planning
ne sera modifié.

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

## Coordination note — shared index race resolved, 2026-07-30

Un chevauchement d'index Git a brièvement regroupé F01 et F04. Il a été résolu
avant push en deux commits ciblés : `747dd30` pour F01 et `2848735` pour F04,
suivis du handoff `969e423`. Le worktree est propre hors CLAIM F02A. Ne plus
réécrire ces commits ; chaque agent vérifie désormais l'index juste avant commit.

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

## Coordination F02A → Agent B, 2026-07-30

Contexte autonome : le contrat HTTP/domain F02A est gelé dans
`docs/contracts/F02A-planning.md`. F02B peut être CLAIM sans attendre la fin de
la review backend ; ses GET/vues restent chez Agent B et les trois POST restent
chez Agent A. Ne pas ajouter `tenant_id` aux formulaires, routes ou DTO.

Point timezone à ne pas deviner : `Day(ctx, time.Time)` reçoit un **instant** et
le store le résout dans le timezone IANA persistant du tenant. Pour un filtre
`day=YYYY-MM-DD`, ne pas utiliser directement `time.Parse(time.DateOnly, ...)`
(minuit UTC peut être la veille en Martinique). Charger d'abord le timezone via
un `Day` courant puis utiliser `time.ParseInLocation`, ou demander une mini-tâche
de contrat si F02B veut un seam dédié.

Le vrai `TodayProvider` planning est maintenant injecté depuis le DI root ;
`FixtureToday` n'est plus utilisé en production. Sa suppression finale touche
deux chemins toujours possédés par Agent B :
`internal/adapters/handlers/dashboard_fixture.go` et le test
`TestFixtureSatisfiesTheContract` dans `dashboard_test.go`. Demande à Agent B :
supprimer ces deux éléments pendant sa prochaine intervention F04/F02B, ou
écrire un handoff explicite autorisant Agent A à le faire. Aucun autre fichier
dashboard/vue/CSS n'est demandé.

## Open handoff — Agent A to Agent B, F02A review, 2026-07-30

Contexte autonome pour review croisée sans historique préalable :

```
Feature: F02A Mini-planning atelier — backend
From: Agent A (backend)
To: Agent B (frontend/reviewer)
Status: REVIEW. Aucun merge vers main demandé ou effectué.

Contrat gelé avant code :
  docs/contracts/F02A-planning.md

Implémentation à reviewer :
  docs/SCHEDULING.md
  internal/core/appointment/**
  internal/adapters/stores/postgres/appointment.go
  internal/adapters/stores/postgres/appointment_test.go
  internal/adapters/stores/postgres/migrations/00003_appointment.sql
  internal/adapters/handlers/appointment*.go
  internal/adapters/httpserver/handler.go + test (routes contractées seulement)
  internal/di/** (wiring seulement)

Garanties :
  - tenant_id vient uniquement du context ; chaque SQL est tenant-scopé ;
  - ouverture persistée obligatoire, aucun horaire inventé ;
  - lock FOR UPDATE + recheck atomique au pic de capacité ;
  - un seul succès concurrent sur le dernier emplacement ;
  - réponse complète du premier write figée en jsonb et rejouée par clé ;
  - FK composites jusque dans appointment_commands ;
  - formulaires bornés, UUID/durée/texte validés, erreurs 401/404/409/422/503 ;
  - vrais RDV adaptés vers F04 ; calls/tasks restent vides sans invention ;
  - ouvertures/RDV traversant minuit correctement bornés au jour du tenant.

Review indépendante : deux findings moyens corrigés (capacité >1 calculée au
pic ; rejeu idempotent complet après reschedule), puis re-review PASS.

Tests/validation :
  TEST_DATABASE_DSN=postgres:18.4... go test -count=1 -race ./...
  go vet ./...
  go build ./...
  docker build -t garage-f02a-test .
  docker compose config --quiet avec variables explicites
  git diff --check
  Base PostgreSQL 18.4 vierge ; migrations et réouverture idempotente.

Point ownership : le vrai provider est câblé, mais FixtureToday + son test sont
encore présents et inutilisés. Ils sont dans dashboard* (owned Agent B). Merci
de les supprimer ou d'autoriser explicitement Agent A dans WORKBOARD.

F02B est débloqué par le contrat. Attention au point time.Time/timezone écrit
dans la coordination juste au-dessus ; ne jamais parser DateOnly en UTC puis
l'utiliser directement comme jour civil Martinique.
```

## Open handoff — Agent A to Agent B, F03 review, 2026-07-30

```
Feature: F03 Voice lookup customer tool
From: Agent A (backend)
To: Agent B (reviewer)
Status: REVIEW. Aucun merge vers main demandé ou effectué.

Contrat gelé avant code : docs/contracts/F03-voice-customer-lookup.md
Documentation officielle vérifiée : docs/ELEVENLABS.md

Implémentation :
  POST /voice/tools/customer-lookup
  Authorization Bearer secret par tenant -> tenant context serveur
  JSON strict/borné : phone uniquement, aucun tenant_id accepté
  connu -> id opaque + prénom seulement ; inconnu -> 200 found=false
  401/422/503 génériques, no-store, aucun secret/SQL/tenant exposé
  VOICE_TOOL_TOKENS optionnel, transmis par Compose

Review indépendante : un finding faible corrigé (UUID tenant canonicalisé avant
détection de doublon de token), puis re-review PASS. Défense supplémentaire :
un store qui renverrait un Customer d'un autre tenant produit 503 sans donnée.

Tests/validation :
  TEST_DATABASE_DSN=PostgreSQL-18.4 go test -count=1 -race ./...
  go vet ./... ; go build ./... ; git diff --check
  docker compose config --quiet avec les variables explicites
  smoke HTTP réel : connu=200 minimal, inconnu=200 found=false, mauvais token=401

Contrainte de déploiement : un agent/tool ElevenLabs par garage reçoit uniquement
le secret de ce garage. Une architecture d'agent partagé exigera une mini-tâche
et une secret dynamic variable, jamais tenant_id ou token dans le prompt LLM.
```

## Open handoff — Agent A to Agent B, F05 review, 2026-07-30

```
Feature: F05 Voice find slots + book appointment
From: Agent A (backend)
To: Agent B (reviewer)
Status: REVIEW. Aucun merge vers main demandé ou effectué.

Contrat gelé avant code : docs/contracts/F05-voice-book-appointment.md
Interfaces provider F02A inchangées.

Implémentation :
  POST /voice/tools/appointment-availability
  POST /voice/tools/appointment-book
  même Bearer secret tenant-scoped que F03 ; tenant_id jamais accepté
  JSON strict/borné ; créneaux issus uniquement du planning PostgreSQL
  idempotency key dérivée côté serveur de system__conversation_id + opération
  confirmed=true uniquement après Book confirmé du tenant authentifié
  horaires convertis explicitement dans le timezone persisté du tenant
  401/422/404/409/503 bornés, no-store, aucune donnée interne exposée

Review indépendante :
  finding moyen 1 corrigé : sortie initialement dépendante du TZ processus ;
  conversion explicite vers le fuseau tenant + smoke sous TZ=UTC ajouté.
  finding moyen 2 corrigé : lecture du fuseau après commit pouvait produire un
  faux confirmed=false ; le fuseau est maintenant validé avant Book et un test
  garantit qu'aucune écriture n'a lieu si cette précondition échoue.
  Re-review finale : PASS.

Validation :
  PostgreSQL 18.4 réel + go test -count=1 -race ./...
  go vet ./... ; go build ./... ; git diff --check
  smoke HTTP réel sous TZ=UTC : slots=200 en -04:00 ; book=200 confirmé en
  -04:00 ; répétition exacte=même appointment ID ; autre opération concurrente
  sur le créneau=409 avec confirmed=false.

Docs officielles vérifiées le 2026-07-30 : Webhook Tools, secret headers,
system__conversation_id et distinction retries event webhook / tool call dans
docs/ELEVENLABS.md. F02B et tous les fichiers UI restent intacts.
```

## Open handoff — Agent B to Agent A, F07 review, 2026-07-30

Contexte autonome pour une review sans historique préalable :

```
Feature: F07 Site public + SEO (PRD §11)
From: Agent B (frontend)
To: Agent A (reviewer)
Status: REVIEW. Aucun merge vers main demandé ou effectué.

Livré (commits `7dd4a24` et `9f05de9`) :
  10 pages SSR : / /fonctionnalites /tarifs /garages /demo /contact
                 /mentions-legales /confidentialite /cgv /cgu
  GET /robots.txt        Disallow /app/ et /voice/ + ligne Sitemap
  GET /sitemap.xml       urlset trié, URLs absolues, ni /app ni /voice
  SEO par page : title, meta description, canonical, Open Graph, JSON-LD
                 Organization minimal (nom + URL, rien d'inventé)

Pourquoi une seule ligne dans internal/adapters/httpserver/handler.go :
  handlers.NewSite().Register(mux). Le site n'a aucune dépendance, donc il n'a
  pas besoin du DI root que tu tenais pour F05. Aucune route, signature ou
  contrat existant modifié. Si tu préfères le construire dans le DI, c'est un
  déplacement d'une ligne : dis-le, je ne l'ai pas mis là par principe.

Ce que le handler garantit, à ne pas casser :
  - "/" est enregistré en GET /{$} : un chemin inconnu fait 404, il ne rend pas
    la page d'accueil. Sans ça un crawler reçoit des doublons infinis en 200.
  - l'URL absolue vient de la requête et de X-Forwarded-Proto, pas d'une config.
    ponytail assumé et commenté : quand le domaine existera, épingler une URL
    absolue en config (ton `internal/config/**`) fermera le sujet du Host forgé.
  - Cache-Control: public, max-age=300 sur les pages, 3600 sur robots/sitemap.
    Ce sont des pages identiques pour tout le monde, aucune donnée tenant.

Rien de tenant, rien de secret : aucune de ces routes ne lit un tenant, une
session ou la base. Le site tourne même si PostgreSQL est à terre.

Chiffres et claims : les prix (349 €/750 min, 599 €/1 750 min) et les alertes
70/85/100 % viennent du PRD §1 et §5. Tout ce qui n'est pas vérifiable
aujourd'hui — identité légale, hébergeur, téléphone, e-mail, numéro de démo —
rend un bloc [À VALIDER] visible sur la page. Aucun témoignage, aucune
référence client, aucune métrique inventée.

Tests / validation :
  go build ./... ; go vet ./... ; go test -race ./... (tout vert)
  12 tests F07 : SEO head par page, 404 hors table, aucun lien interne mort,
    nav/footer alignés sur la table de routes, robots, sitemap trié et absolu,
    canonical qui suit X-Forwarded-Proto, chiffres du PRD présents
  smoke réel : binaire booté contre postgres:18.4-bookworm, les 12 routes
    répondent 200, /nope 404, /app toujours 200, app.css et site.css servis
  visuel : screenshots relus à 1280 px et à 380 px réels, clair et sombre.
    Piège noté au passage : Chrome en vieux headless refuse une fenêtre sous
    ~500 px et *recadre* le screenshot — un faux débordement. Mesuré via une
    iframe de 380 px : clientWidth=380, scrollWidth=380, 0 élément qui dépasse.

Deux défauts trouvés et corrigés avant commit :
  - colonnes du footer étirées par align-items: stretch -> align-items: start ;
  - le CTA bas de /tarifs pointait sur /tarifs ; un test l'interdit maintenant.

Chore finalisé : `dashboard_fixture.go` a été supprimé dans `00f0b2c` et son
  test devenu orphelin pendant le réalignement de branche. Le
  styleguide avait déjà perdu le markup dashboard. Plus aucune fixture morte.

Limites connues, à toi de rien faire :
  - pas d'og:image : aucun visuel de marque n'existe, et une preview cassée est
    pire que pas de preview. Le tag arrivera avec l'asset.
  - pas de formulaire de contact : il n'enverrait rien sans boîte d'envoi. La
    page le dit au visiteur au lieu de mentir.
  - palette de marque toujours [À VALIDER] : l'accent neutre du template.
  - parcours clavier et lecteur d'écran réels : toujours non testés.

Next safe task Agent B : F02B (planning UI). Ton contrat F02A est gelé, mes GET
sont /app/planning et /app/planning/day, tes trois POST restent à toi.
```

## Open handoff — Agent B to Agent A, F02B review, 2026-07-30

Contexte autonome pour une review sans historique préalable :

```
Feature: F02B Mini-planning atelier — UI
From: Agent B (frontend)
To: Agent A (reviewer)
Status: REVIEW. Aucun merge vers main demandé ou effectué. Commit `9e6edfe`.

Zone DI root : RELEASED. Le wiring F02B est fait, F08 peut la prendre.

Contrat consommé sans modification : docs/contracts/F02A-planning.md
  GET /app/planning?day=YYYY-MM-DD                    -> page complète (à moi)
  GET /app/planning/day?day=...&duration_minutes=60   -> fragment (à moi)
  Les trois POST restent les tiens ; je poste dessus depuis des formulaires
  natifs, sans htmx, sans champ inventé.

Le piège timezone que tu avais écrit : traité explicitement.
  Une date civile ne veut rien dire sans le timezone du tenant, et il est en
  base. Donc : Day(ctx, now) d'abord, puis time.ParseInLocation dans la
  location renvoyée, à midi. Jamais time.Parse en UTC utilisé tel quel.
  Un test le verrouille : ?day=2026-07-31 doit arriver chez toi comme le
  2026-07-31 en Martinique, pas comme le 30 à 20:00.

Ce que la page garantit, à ne pas casser :
  - toutes les heures affichées sont converties dans la location du tenant ;
  - une ligne n'est proposée que les créneaux où SA durée entre : une requête
    de disponibilité par durée distincte, bornée à 6, sinon aucune option ;
  - `pending` et `confirmed` seuls reçoivent les boutons déplacer/annuler,
    d'après ta table de transitions ; `in_progress`, `done`, `cancelled`,
    `no_show` n'affichent rien à cliquer ;
  - aucune ouverture en base -> "aucune ouverture enregistrée", jamais un
    08:00-17:00 inventé ;
  - créneaux en erreur mais journée lue -> les RDV s'affichent quand même et
    le panneau dit que les créneaux sont incalculables. Jamais "journée
    complète", qui affirmerait un fait qu'on n'a pas.

Clé d'idempotence, le point à challenger en priorité :
  générée dans la vue, sha256(opération | id | start courant | durée), tronquée.
  Déterministe : double clic ou refresh -> même clé, même donnée -> ton backend
  rejoue sa première réponse. Après un déplacement réussi, le start change donc
  la clé suivante change. Formulaire périmé rejoué depuis le bouton retour ->
  même clé, donnée différente -> 409 chez toi, pas de double booking. Si tu
  préfères une clé fournie par le serveur, c'est une mini-tâche : dis-le.

Incohérence assumée, ton arbitrage :
  sans tenant en contexte, /app/planning répond 401 (table d'erreurs F02A) mais
  /app répond 200 dégradé (contrat F04 gelé, garantie que tu m'as demandée).
  Les deux sont défendables mais divergent. Quand la brique auth/middleware
  existera, il faudra trancher : soit F04 est amendé par mini-tâche, soit le
  planning s'aligne. Aujourd'hui aucune des deux routes n'est exposable
  publiquement, comme ton contrat le dit.

Findings pour toi, dans tes chemins, je n'y touche pas :
  1. internal/adapters/handlers/appointment_today_provider.go passe
     entry.Start tel quel dans la vue F04. Si pgx rend timestamptz en UTC, le
     dashboard affiche des heures UTC. Mon adaptateur convertit explicitement
     avec .In(day.Date.Location()). À vérifier de ton côté ; je n'ai pas pu
     l'observer en vrai puisque /app est dégradé sans tenant.
  2. les trois POST renvoient leurs erreurs en text/plain via http.Error. Ton
     contrat dit "F02B renders human-readable HTML from these outcomes" : après
     un 409 le garage voit "appointment conflict" en Times New Roman. Pour que
     je rende ces cas en HTML il faut soit un redirect avec un motif, soit que
     tu m'appelles pour rendre la page. Mini-tâche à décider ensemble.

Limites connues :
  - pas de formulaire de création de RDV : il faut un customer_id, et F01
    n'expose aucune route HTTP pour chercher un client. Brique manquante, pas
    un oubli. Le jour où elle existe, POST /app/appointments est déjà prêt.
  - pas de vue calendaire (colonnes horaires) : listes + chips. Suffisant pour
    la démo, et ça tient à 380 px.
  - annulation sans double confirmation : elle est repliée dans un <details>,
    donc il faut deux gestes. Pas de dialogue JS.
  - parcours clavier et lecteur d'écran réels : toujours non testés.

Tests / validation :
  go build ./... ; go vet ./... ; go test -race ./... (tout vert)
  16 tests F02B : rendu du jour, timezone du paramètre, date illisible, durée
    refusée, une requête par durée, options limitées à la durée de la ligne,
    statuts terminaux sans action, clés stables et distinctes, aucune ouverture
    inventée, 401 sans tenant, dégradation base, dégradation créneaux seuls,
    fragment sans shell.
  boot réel contre postgres:18.4-bookworm : /app/planning et le fragment
    répondent 401 avec le message honnête, /app et / répondent 200.
  visuel : 1280 px et 380 px réels (clientWidth=380, scrollWidth=380, aucun
    débordement), clair et sombre, <details> replié et déplié.

Next safe task Agent B : F07 reste en review ; sinon UX/a11y ou les pages
  légales à compléter quand le fondateur fournit l'identité.
```

## Open handoff — Agent A to Agent B, F08 review, 2026-07-30

```
Feature: F08 Demande vocale de rappel/devis
From: Agent A (backend)
To: Agent B (reviewer)
Status: REVIEW. Aucun merge vers main demandé ou effectué.

Contrat gelé avant code : docs/contracts/F08-follow-up-request.md
Endpoint : POST /voice/tools/follow-up-request

Garanties :
  Bearer F03 -> tenant context ; aucun tenant_id/customer_id/statut/key LLM
  kind strict callback|quote ; téléphone F01 normalisé ; détails bornés
  client connu rattaché par sous-requête (tenant_id, phone) dans l'INSERT
  numéro connu uniquement dans un autre tenant -> customer_id NULL, sans fuite
  clé DB (tenant, conversation, kind) + hash des champs normalisés
  rejeu identique -> première ligne ; contenu différent -> 409
  recorded=true uniquement après ligne PostgreSQL commise et résultat validé
  réponse minimale ; erreurs 401/422/409/503 no-store sans données sensibles

Review indépendante : PASS pré-wiring puis PASS final. Observation couverte par
un test supplémentaire : deux contenus différents concurrents donnent
exactement une création et un conflit. Le futur changement de statut devra
contracter séparément le rejeu après completed/cancelled ; F08 ne crée que
pending et n'expose aucune mutation de statut.

Validation :
  PostgreSQL 18.4 réel + go test -count=1 -race ./...
  go vet ./... ; go build ./... ; git diff --check
  smoke HTTP réel : 200 enregistré ; rejeu normalisé=même ID ; contenu
  différent=409 recorded=false ; une seule ligne et customer_id tenant correct.

Ownership : aucun fichier UI/CSS/planning modifié. La DI et le numéro de
migration sont libérés après ce passage en REVIEW.
```

## Open handoff — Agent A to Agent B, F09 review, 2026-07-30

```
Feature: F09 Authentification staff et sessions navigateur
From: Agent A (backend)
To: Agent B (frontend/reviewer)
Status: REVIEW. Aucun merge vers main demandé ou effectué.

Contrat gelé avant code : docs/contracts/F09-authentication.md
Routes backend : POST /auth/login, POST /auth/logout
Le GET de connexion reste à la future UI ; aucun fichier templ/CSS/layout touché.

Garanties :
  email normalisé -> staff + tenant persistés, jamais de tenant_id reçu du form
  PBKDF2-HMAC-SHA-256 stdlib, 600k, sel 16 octets, comparaison constant-time
  cookie __Host- Secure/HttpOnly/SameSite=Strict, token aléatoire 32 octets
  seul SHA-256(token) en base, expiration 12h, logout révoqué côté serveur
  /app entier derrière session -> identité staff + tenant injectés ensemble
  http.CrossOriginProtection sur les POST navigateur ; routes voix inchangées
  5 échecs -> lock 15 min ; inconnus font aussi la dérivation anti-enumeration
  budget CPU global mono-instance : 30/min, 2 simultanées, puis 429 Retry-After

Review indépendante :
  HIGH SQLSTATE 42804 sur le CASE de lock trouvé contre PostgreSQL réel, corrigé
  par casts explicites et test resserré ; MEDIUM DoS PBKDF corrigé via MT-05 ;
  LOW exposition du mot de passe/hash dans le seam store corrigé ; re-review PASS.

Validation :
  TEST_DATABASE_DSN=PostgreSQL-18.4 go test -count=1 -race ./...
  parcours DI complet : 401 -> login -> /app tenant-scopé 200 -> logout -> 401
  go vet ./... ; go build ./... ; docker build ; git diff --check

Ownership : les diffs locaux Agent B dans planning_test.go, planning.go,
planning.templ et site_layout.templ sont restés non stagés et hors commit F09.
```

## Review croisée F14 par Agent B — 2026-07-30 — PASS avec deux findings faibles

Contexte autonome : review du webhook post-appel (`c158e29`) contre son propre
contrat `docs/contracts/F14-post-call.md`. Lecture du code, puis vérification
empirique contre PostgreSQL 18.4 réel et l'application bootée avec un secret et
un mapping agent→tenant.

**Vérifié en exécutant, pas en lisant :**

| Cas | Résultat | Verdict |
|---|---|---|
| Signature valide | `200 {"status":"received"}` | ✅ |
| Rejeu identique | `200`, aucun doublon en base | ✅ idempotent |
| Même clé d'événement, contenu différent | `409 event conflict`, la base garde le **premier** contenu | ✅ l'historique n'est pas écrasé |
| Corps altéré avec l'ancienne signature | `401` | ✅ |
| Signature d'un autre secret | `401` | ✅ |
| Horodatage vieux de 31 min | `401` | ✅ tolérance de 30 min respectée |
| En-tête absent | `401` | ✅ |
| Agent inconnu, signature valide | `400 invalid event` | ✅ conforme au contrat (pas `401`) |
| `Content-Type: text/plain` | `415` | ✅ |
| UUID de tenant invalide au démarrage | l'application refuse de démarrer | ✅ conforme |
| `cost_fiat: "0.0715"` | stocké `71500` micro-USD | ✅ exact |
| `cost_fiat: "0.0000005"` | stocké `1` micro-USD | ✅ half-up confirmé |
| `call_successful` absent | `provider_outcome` vide, rien d'inventé | ✅ |

Tests d'intégration relancés avec `TEST_DATABASE_DSN` sur PostgreSQL 18.4 :
`postgres`, `conversation`, `voice`, `di` tous verts, aucun skip.

**Finding 1 — faible : la tolérance d'horodatage n'est pas bornée dans le futur.**
Un `t=` à dix ans dans le futur est accepté (`200`, vérifié). La signature couvre
le timestamp, donc rien n'est forgeable sans le secret, et le risque réel est
mince. Mais un événement légitime signé avec une horloge fournisseur en avance
resterait rejouable indéfiniment, alors que la fenêtre de 30 min existe justement
pour borner ça. Un `timestamp > now+5min → rejet` ferme le sujet, symétriquement.
Ton appel.

**Finding 2 — faible : l'en-tête de signature ne tolère pas d'espace.**
`t=...,` puis ` v0=...` (espace après la virgule) renvoie `401` — vérifié. Le
format documenté aujourd'hui n'a pas d'espace, donc rien n'est cassé ; c'est de la
robustesse si ElevenLabs change son formatage. `strings.TrimSpace(part)` dans la
boucle de parsing suffit.

**Nit, aucune action requise :** `strings.NewReader(string(rawBody))` copie le
corps en string deux fois dans `decodePostCallEvent` ; `bytes.NewReader(rawBody)`
fait la même chose sans la copie. Deux occurrences.

**Rien à changer sur le reste.** Signature vérifiée avant tout parsing, corps
brut signé et jamais réencodé, secret jamais loggé, aucune donnée fournisseur ni
tenant dans les réponses d'erreur, tenant résolu côté serveur depuis le mapping,
`cost` legacy laissé dans le JSON brut sans interprétation.

Pour ma F15 : les colonnes `started_at`, `duration_seconds`, `summary`,
`provider_outcome`, `provider_status` et `transcript` couvrent exactement mes DTO.
L'adaptateur MT-09 est un mapping mince, pas une nouvelle requête à concevoir.


## SERIAL zones
Current owner must be written here before edits.

| Zone | Owner | Until | Reason |
|---|---|---|---|
| `go.mod` / `go.sum` | Agent A | F00 MERGED | migration SQLite vers pgx/Goose |
| `go.mod` / `go.sum` — ajout templ | Agent B — **RELEASED** | fait le 2026-07-30 | `github.com/a-h/templ v0.3.1020` ajouté sur autorisation explicite du fondateur, agent A absent. Une seule dépendance, ancrée par `internal/web/views/layout.templ` (sinon `go mod tidy` la supprime). Rien d'autre touché : DI root et routes intacts. |
| DI root | - | - | F18 câblé ; zone libérée après review PASS |
| DB migration numbering | - | - | migration F09 `00005_authentication.sql` terminée ; prochain owner doit CLAIM |
| `compose.yml` | - | - | F03 terminé ; prochain owner doit CLAIM avant édition |
| `Dockerfile` | Agent A | F00 MERGED | supprimer les hypothèses SQLite de l'image applicative |
| global layout/tokens | Agent B | F06 MERGED | global UI contract |
| provider interfaces | - | - | `SchedulingProvider` F02A gelé ; mini-tâche obligatoire avant changement |

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
