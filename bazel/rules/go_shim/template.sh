#!/usr/bin/env bash

set -euo pipefail

cd "$BUILD_WORKING_DIRECTORY"
GOROOT="$OLDPWD/{{goroot}}" GOTOOLCHAIN=local PATH="$OLDPWD/{{go_dir}}:$PATH" exec "$OLDPWD/{{tool}}" "$@"
