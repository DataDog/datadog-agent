#!/usr/bin/env bash
set -euo pipefail

# Bazel credential helper for Buildbarn Vault OIDC authentication
# Protocol: https://github.com/bazelbuild/proposals/blob/main/designs/2022-06-07-bazel-credential-helpers.md
# - Receives JSON request on stdin with URI
# - Outputs JSON response with headers

VAULT_ADDR="${VAULT_ADDR:-https://vault.us1.ddbuild.staging.dog}"
CACHE_DIR="${HOME}/.cache/buildbarn"
TOKEN_FILE="${CACHE_DIR}/vault-token"

fetch_token() {
  vault read -address="$VAULT_ADDR" -field=token identity/oidc/token/buildbarn > "$TOKEN_FILE" 2>/dev/null
}

ensure_token() {
  mkdir -p "$CACHE_DIR"
  # Refresh if missing or older than 55 minutes (token TTL is 1 hour)
  if [[ ! -f "$TOKEN_FILE" ]] || [[ $(find "$TOKEN_FILE" -mmin +55 2>/dev/null) ]]; then
    if ! fetch_token; then
      echo "Vault token missing or expired, logging in..." >&2
      vault login -address="$VAULT_ADDR" -method=oidc >&2 || { echo '{"error":"Vault login failed"}' >&2; exit 1; }
      fetch_token || { echo '{"error":"Failed to fetch Vault OIDC token after login"}' >&2; exit 1; }
    fi
  fi
}

# Read JSON request from stdin and extract URI
uri=$(cat | grep -o '"uri":"[^"]*"' | sed 's/"uri":"\([^"]*\)"/\1/')

if [[ "$uri" == *"buildbarn"* ]]; then
  ensure_token
  printf '{\n  "headers": {\n    "Authorization": ["Bearer %s"]\n  }\n}\n' "$(cat "$TOKEN_FILE")"
else
  echo "{}"
fi
