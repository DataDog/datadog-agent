#!/usr/bin/env bash
set -euo pipefail

install_dir="$(dirname "$(dirname "$1")")"
shift

lib_dirs=""
for lib in "$@"; do
    lib_dirs="${lib_dirs:+${lib_dirs}:}$(cd "$(dirname "${lib}")" && pwd)"
done

LD_LIBRARY_PATH="${lib_dirs}" "${install_dir}/bin/python${PYTHON_MAJOR_MINOR}" -c "
import importlib, sys
for name in '${MODULES}'.split():
    importlib.import_module(name)
import json, ssl, hashlib, sqlite3, decimal, ctypes, zlib, lzma, bz2, xml.etree.ElementTree, subprocess
assert json.loads('{\"a\": 1}') == {'a': 1}
assert hashlib.sha256(b'').hexdigest().startswith('e3b0c442')
assert hashlib.md5(b'').hexdigest() == 'd41d8cd98f00b204e9800998ecf8427e'
assert sqlite3.connect(':memory:').execute('select 1').fetchone() == (1,)
assert decimal.Decimal('1.1') + decimal.Decimal('2.2') == decimal.Decimal('3.3')
assert ctypes.CDLL(None).getpid() > 0
assert zlib.decompress(zlib.compress(b'x')) == b'x'
assert lzma.decompress(lzma.compress(b'x')) == b'x'
assert bz2.decompress(bz2.compress(b'x')) == b'x'
assert xml.etree.ElementTree.fromstring('<a/>').tag == 'a'
assert subprocess.run(['true']).returncode == 0
print(sys.version, ssl.OPENSSL_VERSION)
"
