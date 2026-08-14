#!/usr/bin/env bash

set -euo pipefail

cd "$BUILD_WORKING_DIRECTORY"
PATH="$OLDPWD/{{go_dir}}:$PATH" RUNFILES_DIR="$OLDPWD/.." exec "$OLDPWD/{{tool}}" "$@"
