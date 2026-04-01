#!/usr/bin/env bash
set -euo pipefail

# Credential helper wrapper for Bazel.
# Reads a JSON request from stdin, extracts the URI, and delegates to
# the Tweag credential helper binary when appropriate.

request=$(cat)
uri=$(echo "$request" | sed -n 's/.*"uri" *: *"\([^"]*\)".*/\1/p' | head -1)

# Build the bearer token from the CI id_token for the tweag credential helper
if [[ -n "${BUILDBARN_ID_TOKEN:-}" ]]; then
  export BUILDBARN_BEARER_TOKEN="Bearer ${BUILDBARN_ID_TOKEN}"
fi

# Skip auth for localhost URLs (e.g. BEP)
case "${uri:-}" in
  http://localhost*|https://localhost*|http://127.0.0.1*|https://127.0.0.1*)
    echo '{"headers":{}}'
    exit 0
    ;;
esac

# Determine platform-appropriate cache directory for the credential helper binary
if [[ "$OSTYPE" == darwin* ]]; then
  helper_bin="$HOME/Library/Caches/bazel/credential-helper/credential-helper"
else
  helper_bin="${HOME}/.cache/bazel/credential-helper/credential-helper"
fi

# If the binary is not found, return empty headers (non-fatal)
if [[ ! -x "$helper_bin" ]]; then
  echo '{"headers":{}}'
  exit 0
fi

# Delegate to the real credential helper
echo "$request" | exec "$helper_bin" "$@"
