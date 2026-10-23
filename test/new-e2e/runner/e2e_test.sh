#!/bin/sh

set -eu

case " ${E2E_TEST_PACKAGES:-{{importpath}}} " in  # unset or empty selects every package
*" {{importpath}} "*) ;;
*) exit 0 ;;
esac
cd "$BUILD_WORKSPACE_DIRECTORY/{{package}}"
GOROOT="$OLDPWD/{{goroot}}" GOTOOLCHAIN=local exec "$OLDPWD/{{go}}" tool test2json -p "{{importpath}}" -t \
    "$OLDPWD/{{test}}" -test.v=test2json "$@"
