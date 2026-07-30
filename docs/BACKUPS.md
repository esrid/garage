# Sauvegardes

Le PRD (§12) demande une sauvegarde quotidienne de PostgreSQL vers un stockage
externe au VPS. Ce document décrit ce qui est en place, ce qui ne l'est pas, et
le compromis assumé.

## Ce qui est livré

- `ops/backup.sh` — un dump logique quotidien, format custom, vérifié puis
  tourné.
- `ops/restore.sh` — la restauration, à exécuter **avant** d'en avoir besoin.

## Le compromis, et quand le revoir

`pg_dump` produit un export **cohérent** d'une base en service : il prend un
instantané, ne bloque personne, n'exige aucune interruption. Vérifié dans la
documentation officielle (`https://www.postgresql.org/docs/18/app-pgdump.html`,
consultée le 2026-07-30), qui dit aussi clairement que ce n'est **pas** de la
restauration à un instant donné (PITR) et qu'au-delà des cas simples, la
sauvegarde WAL est la bonne réponse.

Ce que ça veut dire concrètement : **on peut perdre jusqu'à une journée
d'appels et de rendez-vous.** C'est le compromis, et il est délibéré au stade
actuel — un dump quotidien qu'on restaure réellement vaut mieux qu'une
configuration WAL que personne ne vérifie.

À revoir dès que l'une de ces phrases devient vraie :

- perdre une journée de rendez-vous coûterait plus qu'une demi-journée de mise
  en place ;
- il y a plus d'un atelier en production ;
- la base dépasse quelques centaines de mégaoctets, où le dump commence à peser
  sur la nuit.

La suite est alors `pg_basebackup` + archivage WAL, ou le service managé du
fournisseur.

## Ce que le script refuse de faire

- **Garder un dump trop petit.** Un fichier sous 20 Ko signifie que le schéma a
  disparu, pas que l'atelier a eu une journée calme.
- **Garder un dump illisible.** Chaque dump est relu avec `pg_restore --list`
  juste après son écriture : un fichier qu'on ne peut pas lister, on ne pourra
  pas le restaurer non plus, et le découvrir pendant un incident est trop tard.
- **Tourner avant d'avoir réussi.** La rotation ne s'exécute qu'après un dump
  écrit et vérifié : supprimer d'abord échangerait un historique complet contre
  une exécution ratée.

Toute défaillance sort en code non nul, pour que cron ou un timer systemd la
signale au lieu de l'avaler.

## Planifier

Depuis l'hôte, une fois par nuit, avec le stockage externe monté :

```cron
15 3 * * * DATABASE_DSN=postgres://... BACKUP_DIR=/mnt/backups/garage /opt/garage/ops/backup.sh >> /var/log/garage-backup.log 2>&1
```

`BACKUP_DIR` **doit** pointer sur un volume qui n'est pas celui de la base :
une sauvegarde sur le même disque disparaît avec lui. Le PRD demande un
stockage externe au VPS ; tant qu'il n'est pas choisi, cette exigence n'est pas
satisfaite et ce fichier doit le dire.

## Restaurer

```sh
DATABASE_DSN=postgres://.../garage_restore_test ops/restore.sh /mnt/backups/garage/garage-20260730T221307Z.dump
```

Le script **refuse** un DSN dont le nom ne contient pas `restore`, `test` ou
`staging` : la commande qui répare est aussi celle qui écrase, et elle ne doit
pas pouvoir viser la production par inattention.

## Vérifié le 2026-07-30

- dump d'une base réelle : 56 052 octets, contrôle de taille et
  `pg_restore --list` passés ;
- restauration dans une base neuve : 133 ateliers, 104 clients, 12 rendez-vous,
  60 appels, 24 comptes staff retrouvés ;
- **non vérifié** : le démarrage de l'application sur la base restaurée. Le
  démon Docker de la machine de développement s'est arrêté pendant l'essai. À
  refaire avant de considérer la restauration comme prouvée.

## Ce qui n'est pas sauvegardé

- Le fichier `.env` et les secrets (jeton voix, secret webhook, mot de passe de
  la base) : ils vivent hors du dump, et une restauration sans eux redonne des
  données que l'application ne peut pas servir.
- Rien d'autre : l'application ne stocke aucun fichier, les gabarits et les
  assets sont dans l'image.
