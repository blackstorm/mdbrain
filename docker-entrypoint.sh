#!/usr/bin/env sh
set -eu

if [ "$#" -gt 0 ]; then
  exec "$@"
fi

echo "Applying database migrations..."
/app/mdbrain-migrate migrate

exec /app/mdbrain
