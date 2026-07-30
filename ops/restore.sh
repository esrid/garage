#!/bin/sh
# Restore a dump produced by ops/backup.sh into a database.
#
# Its real job is to be run on purpose, in the quiet, before it is needed: a
# backup nobody has restored is a file, not a backup.
#
#   DATABASE_DSN=postgres://.../garage_restore_test ops/restore.sh backup.dump
#
# --clean --if-exists drops what it replaces, so the target must be a database
# you accept losing. It refuses to run against a DSN whose name has no marker.
set -eu

: "${DATABASE_DSN:?set DATABASE_DSN}"
dump="${1:?usage: restore.sh <dump file>}"

case "$DATABASE_DSN" in
    *restore*|*test*|*staging*) ;;
    *)
        echo "restore: refusing a DSN that does not name a restore, test or staging database" >&2
        echo "restore: point it at a throwaway database, never at production" >&2
        exit 1
        ;;
esac

pg_restore --dbname="$DATABASE_DSN" --clean --if-exists --no-owner --single-transaction "$dump"
echo "restore: $dump restored"
