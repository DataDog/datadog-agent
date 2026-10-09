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
install="$TEST_TMPDIR/python"

cp -rL "$(rlocation "$PYTHON_INSTALL")" "$install"
chmod -R u+w "$install"

out="$("$runner" "$install/python.exe" -c '
import importlib
modules = [
    "_overlapped", "_wmi", "asyncio", "bz2", "ctypes", "decimal", "hashlib",
    "json", "lzma", "multiprocessing", "pyexpat", "queue", "select", "socket",
    "sqlite3", "ssl", "unicodedata", "uuid", "winsound", "xml.etree.ElementTree",
    "zlib", "zoneinfo",
]
for name in modules:
    importlib.import_module(name)
import ctypes, decimal, hashlib, sqlite3, ssl, zlib
assert ssl.OPENSSL_VERSION.startswith("OpenSSL 3."), ssl.OPENSSL_VERSION
ssl.create_default_context()
assert hashlib.sha256(b"x").hexdigest().startswith("2d711642b726b044")
assert zlib.decompress(zlib.compress(b"x" * 100)) == b"x" * 100
assert sqlite3.connect(":memory:").execute("select 1").fetchone() == (1,)
sqlite3.connect(":memory:").execute("create virtual table t using fts5(x)")
assert str(decimal.Decimal(1) / 4) == "0.25"
assert ctypes.windll.kernel32.GetCurrentProcessId() > 0
print("OK", ssl.OPENSSL_VERSION)
')"
echo "$out"
[[ "$out" == OK\ OpenSSL\ 3.* ]]
