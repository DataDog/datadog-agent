#!/bin/bash
# Hermetic checks against fake Agent installs, built from a real Mozilla
# extract (CACERT_SRC, default /tmp/cacert-latest.pem) plus the fixtures.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
FIX=$ROOT/tests/fixtures
PY=${PYTHON:-python3}
SRC=${CACERT_SRC:-/tmp/cacert-latest.pem}
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

if [ ! -f "$SRC" ]; then
  echo "missing $SRC" >&2
  exit 1
fi

run() { "$PY" "$ROOT/update_agent_cacert.py" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# Every installed bundle carries a distrusted root and a CA added on the host,
# whatever the source extract contains.
ORIG=$WORK/orig.pem
cat "$SRC" "$FIX/emsign-root-ca-g1.crt" "$FIX/custom-ca.crt" > "$ORIG"

install=$WORK/opt/datadog-agent
site=$install/embedded/lib/python3.12/site-packages
mkdir -p "$install/embedded/ssl/certs" "$site/certifi" "$site/pip/_vendor/certifi" "$site/botocore" "$site/vendorlib"
SHIPPED=(
  "$install/embedded/ssl/certs/cacert.pem"
  "$site/certifi/cacert.pem"
  "$site/pip/_vendor/certifi/cacert.pem"
  "$site/botocore/cacert.pem"
)
for f in "${SHIPPED[@]}"; do cp "$ORIG" "$f"; done
chmod 0640 "$install/embedded/ssl/certs/cacert.pem"
ln -s certs/cacert.pem "$install/embedded/ssl/cert.pem"
# A bundle from a package the Agent does not ship must never be written.
cp "$ORIG" "$site/vendorlib/cacert.pem"

check() {
  "$PY" - "$ROOT" "$SRC" "$ORIG" "$FIX/custom-ca.crt" "$@" <<'PY'
import sys
sys.path.insert(0, sys.argv[1])
import update_agent_cacert as u

src, orig, custom, mode, path = sys.argv[2:7]
read = lambda p: open(p, encoding="latin-1").read()
got = read(path)
if mode == "scrubbed":
    assert u.fingerprints(got) == u.fingerprints(read(orig)) - set(u.DISTRUST), path
    assert u.fingerprints(read(custom)) <= u.fingerprints(got), "custom CA lost"
elif mode == "latest":
    assert got == u.clean_source(read(src))[0], path
PY
}

echo "=== refusals ==="
mkdir "$WORK/empty"
if run -n -p "$WORK/empty"; then fail "empty install accepted"; fi
if run -n -c "$SRC" -p "$install"; then fail "--bundle accepted without --latest"; fi
BAD=$WORK/bad.pem
{ cat "$SRC"; echo "-----BEGIN CERTIFICATE-----"; printf 'not a certificate' | base64; echo "-----END CERTIFICATE-----"; } > "$BAD"
if run --latest -c "$BAD" -p "$install"; then fail "corrupt source accepted"; fi
for f in "${SHIPPED[@]}"; do cmp -s "$ORIG" "$f" || fail "$f changed by a refused run"; done
if ls "$install/embedded/ssl/certs"/cacert.pem.pre-cert-update-* 2>/dev/null; then fail "backup written by a refused run"; fi

echo "=== scrub dry run ==="
out=$(run -n -p "$install")
echo "$out"
echo "$out" | grep -q "4 bundle(s) would be replaced, 1 skipped"
echo "$out" | grep -q "not touched (not shipped with the Agent) $site/vendorlib/cacert.pem"
for f in "${SHIPPED[@]}"; do cmp -s "$ORIG" "$f"; done

echo "=== scrub ==="
run -p "$install"
for f in "${SHIPPED[@]}"; do check scrubbed "$f"; done
mode=$("$PY" -c 'import os,sys;print(oct(os.stat(sys.argv[1]).st_mode & 0o777)[2:])' "$install/embedded/ssl/certs/cacert.pem")
[ "$mode" = "640" ] || fail "mode $mode"
[ -L "$install/embedded/ssl/cert.pem" ] || fail "symlink replaced"
cmp -s "$ORIG" "$site/vendorlib/cacert.pem" || fail "vendor bundle written"
run -p "$install" | grep -q "0 bundle(s) replaced"

echo "=== rollback ==="
run -n -r -p "$install" | grep -q "4 bundle(s) would be restored"
run -r -p "$install"
for f in "${SHIPPED[@]}"; do cmp -s "$ORIG" "$f" || fail "$f not restored"; done

echo "=== --latest ==="
out=$(run --latest -n -c "$SRC" -p "$install")
echo "$out"
echo "$out" | grep -q "drops (not in the extract): Example Corp Proxy CA (test fixture)"
echo "$out" | grep -q "drops (distrusted): emSign Root CA - G1"
run --latest -c "$SRC" -p "$install"
for f in "${SHIPPED[@]}"; do check latest "$f"; done
cmp -s "$ORIG" "$site/vendorlib/cacert.pem" || fail "vendor bundle written"
run -r -p "$install"
for f in "${SHIPPED[@]}"; do cmp -s "$ORIG" "$f" || fail "$f not restored after --latest"; done

echo "=== Windows layout ==="
win=$WORK/win
mkdir -p "$win/embedded3/ssl/certs" "$win/embedded3/Lib/site-packages/certifi"
cp "$ORIG" "$win/embedded3/ssl/cert.pem"
cp "$ORIG" "$win/embedded3/ssl/certs/cacert.pem"
cp "$ORIG" "$win/embedded3/Lib/site-packages/certifi/cacert.pem"
run -p "$win" | grep -q "3 bundle(s) replaced, 0 skipped"
check scrubbed "$win/embedded3/ssl/cert.pem"

echo "ok"
