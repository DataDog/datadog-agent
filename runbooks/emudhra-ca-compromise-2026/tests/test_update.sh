#!/bin/bash
# Hermetic checks against a fake Agent install, built from a real Mozilla
# extract (CACERT_SRC, default /tmp/cacert-latest.pem).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
PY=${PYTHON:-python3}
SRC=${CACERT_SRC:-/tmp/cacert-latest.pem}
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

if [ ! -f "$SRC" ]; then
  echo "missing $SRC" >&2
  exit 1
fi

install=$WORK/opt/datadog-agent
mkdir -p "$install/embedded/ssl/certs" \
  "$install/embedded/lib/python3.12/site-packages/certifi" \
  "$install/embedded/lib/python3.12/site-packages/pip/_vendor/certifi" \
  "$install/embedded/ssl/certs/not-a-store"

# The trailing line makes every installed bundle differ from the cleaned
# result, even when the source has no distrusted roots left.
ORIG=$WORK/orig.pem
{ cat "$SRC"; echo "# stale"; } > "$ORIG"
cp "$ORIG" "$install/embedded/ssl/certs/cacert.pem"
cp "$ORIG" "$install/embedded/ssl/cert.pem"
cp "$ORIG" "$install/embedded/lib/python3.12/site-packages/certifi/cacert.pem"
cp "$ORIG" "$install/embedded/lib/python3.12/site-packages/pip/_vendor/certifi/cacert.pem"
chmod 0640 "$install/embedded/ssl/certs/cacert.pem"

# A file with too few certificates is not a trust store and must be left alone.
head -n 30 "$SRC" > "$install/embedded/ssl/certs/not-a-store/cacert.pem"
tiny_before=$(cksum "$install/embedded/ssl/certs/not-a-store/cacert.pem")

# A symlink must not be rewritten, and its target must not change.
echo "outside" > "$WORK/outside.pem"
ln -s "$WORK/outside.pem" "$install/embedded/ssl/certs/linked-cacert.pem"
# The walker keys on the filename, so the symlink has to be one of the bundle names.
rm "$install/embedded/ssl/certs/linked-cacert.pem"
ln -s "$WORK/outside.pem" "$install/embedded/lib/python3.12/site-packages/certifi/ca-certificates.crt"

# The expected result is derived from the source, so any Mozilla extract works.
check_cleaned() {
  "$PY" - "$ROOT" "$SRC" "$1" <<'PY'
import sys
sys.path.insert(0, sys.argv[1])
import update_agent_cacert as u

def fps(path):
    text = open(path, encoding="latin-1").read()
    return [u.fingerprint(b) for b in u.BLOCK.findall(text)]

want = {fp for fp in fps(sys.argv[2]) if fp not in u.DISTRUST}
got = fps(sys.argv[3])
assert not set(got) & set(u.DISTRUST), "distrusted root still present"
assert set(u.REQUIRED) <= set(got), "anchor missing"
assert set(got) == want and len(got) == len(want), (len(got), len(want))
PY
}

echo "=== a source OpenSSL cannot load is refused ==="
BAD=$WORK/bad.pem
{ cat "$SRC"; echo "-----BEGIN CERTIFICATE-----"; printf 'not a certificate' | base64; echo "-----END CERTIFICATE-----"; } > "$BAD"
if "$PY" "$ROOT/update_agent_cacert.py" -c "$BAD" -p "$install"; then
  echo "corrupt source was accepted" >&2
  exit 1
fi
cmp -s "$ORIG" "$install/embedded/ssl/certs/cacert.pem"
if ls "$install/embedded/ssl/certs"/cacert.pem.pre-cert-update-* 2>/dev/null; then
  echo "backup written for a refused source" >&2
  exit 1
fi

echo "=== dry run ==="
out=$("$PY" "$ROOT/update_agent_cacert.py" -n -c "$SRC" -p "$install")
echo "$out"
echo "$out" | grep -q "4 bundle(s) would be replaced"
cmp -s "$ORIG" "$install/embedded/ssl/certs/cacert.pem"

echo "=== apply ==="
"$PY" "$ROOT/update_agent_cacert.py" -c "$SRC" -p "$install"
mode=$("$PY" -c 'import os,sys;print(oct(os.stat(sys.argv[1]).st_mode & 0o777)[2:])' "$install/embedded/ssl/certs/cacert.pem")
[ "$mode" = "640" ]
check_cleaned "$install/embedded/ssl/certs/cacert.pem"
[ "$(cksum "$install/embedded/ssl/certs/not-a-store/cacert.pem")" = "$tiny_before" ]
[ "$(cat "$WORK/outside.pem")" = "outside" ]
ls "$install/embedded/ssl/certs"/cacert.pem.pre-cert-update-* >/dev/null
ls "$install/embedded/ssl"/cert.pem.pre-cert-update-* >/dev/null

echo "=== second apply is a no-op ==="
out=$("$PY" "$ROOT/update_agent_cacert.py" -c "$SRC" -p "$install")
echo "$out" | grep -q "0 bundle(s) replaced"

echo "=== rollback dry run then rollback ==="
"$PY" "$ROOT/update_agent_cacert.py" -n -r -p "$install" | grep -q "would be restored"
"$PY" "$ROOT/update_agent_cacert.py" -r -p "$install"
cmp -s "$ORIG" "$install/embedded/ssl/certs/cacert.pem"
cmp -s "$ORIG" "$install/embedded/ssl/cert.pem"

echo "ok"
