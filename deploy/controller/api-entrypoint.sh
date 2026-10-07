#!/bin/sh
set -eu
if [ ! -r /run/secrets/database-url ]; then
  echo 'database credential file is not readable' >&2
  exit 1
fi
DATABASE_URL=$(cat /run/secrets/database-url)
export DATABASE_URL
exec /usr/local/bin/mealcheck-server -root /opt/mealcheck "$@"
