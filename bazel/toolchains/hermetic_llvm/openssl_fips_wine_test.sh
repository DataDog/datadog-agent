#!/usr/bin/env bash

# --- begin runfiles.bash initialization v3 ---
set -uo pipefail; set +e; f=bazel_tools/tools/bash/runfiles/runfiles.bash
# shellcheck disable=SC1090
source "${RUNFILES_DIR:-/dev/null}/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "${RUNFILES_MANIFEST_FILE:-/dev/null}" | cut -f2- -d' ')" 2>/dev/null || \
  source "$0.runfiles/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.exe.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  { echo>&2 "ERROR: cannot find $f"; exit 1; }; f=; set -e
# --- end runfiles.bash initialization v3 ---

runner="$(rlocation "$WINE_RUN")"

cd "$TEST_TMPDIR"
for f in $OPENSSL_FILES; do
    cp "$(rlocation "$f")" .
done
cp "$(rlocation "$FIPS_PROVIDER")" .

# Runs the module self-tests and records its integrity MAC. Without a path
# separator, OpenSSL would append .dll to the module name.
"$runner" "$PWD/openssl.exe" fipsinstall -module '.\fips.dll' -out fipsmodule.cnf

cat >openssl.cnf <<'EOF'
config_diagnostics = 1
openssl_conf = openssl_init

.include fipsmodule.cnf

[openssl_init]
providers = provider_sect

[provider_sect]
fips = fips_sect
EOF

out="$(OPENSSL_CONF=openssl.cnf OPENSSL_MODULES=. "$runner" "$PWD/openssl.exe" list -providers)"
if [[ "$out" != *"OpenSSL FIPS Provider"* || "$out" != *"status: active"* ]]; then
    echo "FIPS provider not active: $out" >&2
    exit 1
fi
