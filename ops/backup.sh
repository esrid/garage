#!/bin/sh
# Daily PostgreSQL backup for one workshop deployment (PRD §12).
#
# pg_dump is a consistent logical export of a live database: it takes a snapshot,
# blocks nobody, and needs no downtime. It is not point-in-time recovery, and the
# PostgreSQL documentation says so plainly - the WAL-based approach is the answer
# when losing a day is not acceptable. At the scale this product starts at, a
# daily dump that is actually restored and tested beats a WAL setup that nobody
# verifies. docs/BACKUPS.md records that trade-off and when to revisit it.
#
# Verified against https://www.postgresql.org/docs/18/app-pgdump.html, 2026-07-30.
#
#   DATABASE_DSN=postgres://... BACKUP_DIR=/var/backups/garage ops/backup.sh
#
# Exits non-zero on any failure so a cron job or a systemd timer reports it.
set -eu

: "${DATABASE_DSN:?set DATABASE_DSN}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/garage}"
KEEP_DAYS="${KEEP_DAYS:-30}"
# A dump smaller than this means the schema went missing, not that the workshop
# had a quiet day. Empirically an empty schema-only dump is a few kilobytes.
MIN_BYTES="${MIN_BYTES:-20000}"

mkdir -p "$BACKUP_DIR"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
target="$BACKUP_DIR/garage-$stamp.dump"

# -Fc: the custom format, compressed, restorable selectively and in parallel with
# pg_restore. Plain SQL is only worth it to read the dump by hand.
pg_dump --dbname="$DATABASE_DSN" --format=custom --file="$target"

size=$(wc -c < "$target")
if [ "$size" -lt "$MIN_BYTES" ]; then
    echo "backup: $target is only $size bytes, refusing to keep it" >&2
    rm -f "$target"
    exit 1
fi

# Read the dump back with pg_restore --list. A file that cannot be listed cannot
# be restored either, and finding that out during an incident is too late.
if ! pg_restore --list "$target" > /dev/null 2>&1; then
    echo "backup: $target is not a readable dump, refusing to keep it" >&2
    rm -f "$target"
    exit 1
fi

# Rotate only after a new dump has been written and checked: deleting first would
# trade a full history for a failed run.
find "$BACKUP_DIR" -name 'garage-*.dump' -type f -mtime "+$KEEP_DAYS" -delete

echo "backup: $target ($size bytes), keeping $KEEP_DAYS days"
