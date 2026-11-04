#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
if [ "$#" != 1 ]; then
  printf 'Usage: sudo sh rollback.sh /var/backups/corade/offline-TIMESTAMP-PID\n' >&2
  exit 1
fi
exec sh "$ROOT/upgrade.sh" --rollback "$1"
