#!/bin/sh
set -eu
exec sh "$(dirname "$0")/installer-signature-test.sh" "${1:-./install.sh}"
