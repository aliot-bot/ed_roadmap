#!/bin/sh
set -eu

backup_dir=${BACKUP_DIR:-./backups}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
umask 077
mkdir -p "$backup_dir"
output="$backup_dir/knowledge-map-$timestamp.dump"
temporary="$output.partial.$$"

if [ -e "$output" ]; then
  printf '%s\n' "Refusing to overwrite existing backup: $output" >&2
  exit 73
fi

cleanup() {
  rm -f "$temporary"
}
trap cleanup 0 HUP INT TERM

docker compose exec -T postgres sh -eu -c \
  'exec pg_dump --format=custom --no-owner --no-acl --username="$POSTGRES_USER" --dbname="$POSTGRES_DB"' \
  >"$temporary"

test -s "$temporary"
docker compose exec -T postgres pg_restore --list <"$temporary" >/dev/null
mv "$temporary" "$output"
trap - 0 HUP INT TERM
printf '%s\n' "Backup created: $output"
