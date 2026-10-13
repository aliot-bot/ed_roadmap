#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  printf '%s\n' "Usage: RESTORE_DATABASE=<database> $0 <backup.dump>" >&2
  exit 64
fi

backup_file=$1
restore_database=${RESTORE_DATABASE:-knowledge_map_restore_check}

if [ ! -f "$backup_file" ] || [ ! -r "$backup_file" ]; then
  printf '%s\n' "Backup must be a readable regular file: $backup_file" >&2
  exit 66
fi

case "$restore_database" in
  ''|*[!a-zA-Z0-9_]* )
    printf '%s\n' "RESTORE_DATABASE may contain only letters, digits, and underscores" >&2
    exit 64
    ;;
esac

running_database=$(docker compose exec -T postgres sh -eu -c 'printf %s "$POSTGRES_DB"')
database_user=$(docker compose exec -T postgres sh -eu -c 'printf %s "$POSTGRES_USER"')
if [ "$restore_database" = "$running_database" ] && [ "${CONFIRM_PRODUCTION_RESTORE:-}" != "RESTORE_EXISTING_DATABASE" ]; then
  printf '%s\n' "Refusing to overwrite the configured database without CONFIRM_PRODUCTION_RESTORE=RESTORE_EXISTING_DATABASE" >&2
  exit 77
fi

# Validate the archive before creating or replacing any database.
docker compose exec -T postgres pg_restore --list <"$backup_file" >/dev/null

if docker compose exec -T postgres psql --username="$database_user" --dbname=postgres -tAc \
  "SELECT 1 FROM pg_database WHERE datname = '$restore_database'" | grep -q 1; then
  if [ "${CONFIRM_REPLACE_RESTORE_DATABASE:-}" != "REPLACE_RESTORE_DATABASE" ]; then
    printf '%s\n' "Database $restore_database already exists; set CONFIRM_REPLACE_RESTORE_DATABASE=REPLACE_RESTORE_DATABASE to replace it" >&2
    exit 77
  fi
  docker compose exec -T postgres dropdb --force --username="$database_user" "$restore_database"
fi

docker compose exec -T postgres createdb --username="$database_user" "$restore_database"
if ! docker compose exec -T postgres pg_restore --exit-on-error --no-owner --no-acl \
  --username="$database_user" --dbname="$restore_database" <"$backup_file"; then
  printf '%s\n' "Restore failed; inspect and remove database $restore_database manually" >&2
  exit 1
fi

docker compose exec -T postgres psql --username="$database_user" --dbname="$restore_database" \
  -c 'SELECT current_database(), now();'
printf '%s\n' "Restore verified in database: $restore_database"
