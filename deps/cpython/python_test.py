"""Smoke test for the installed CPython.

Imports every module the build produces, like upstream's `make checksharedmods`
(Tools/build/check_extension_modules.py), which cannot run here as it reads the
module list from the Makefile and Modules/Setup files. Modules backed by an
external library also get a functional check.
"""

import importlib
import sys


def main(modules_file):
    with open(modules_file) as f:
        modules = f.read().split()

    for name in modules:
        importlib.import_module(name)

    import binascii
    import bz2
    import ctypes
    import decimal
    import json
    import lzma
    import sqlite3
    import ssl
    import subprocess
    import xml.etree.ElementTree
    import zlib

    import _hashlib
    import _md5
    import _sha2

    checks = {
        "_bz2": lambda: bz2.decompress(bz2.compress(b"x")) == b"x",
        "_ctypes": lambda: ctypes.CDLL(None).getpid() > 0,
        "_decimal": lambda: decimal.Decimal("1.1") + decimal.Decimal("2.2") == decimal.Decimal("3.3"),
        "_hashlib": lambda: _hashlib.new("sha256", b"").hexdigest().startswith("e3b0c442"),
        "_json": lambda: json.loads('{"a": 1}') == {"a": 1},
        "_lzma": lambda: lzma.decompress(lzma.compress(b"x")) == b"x",
        "_md5": lambda: _md5.md5(b"").hexdigest() == "d41d8cd98f00b204e9800998ecf8427e",
        "_posixsubprocess": lambda: subprocess.run([sys.executable, "-c", "pass"]).returncode == 0,
        "_sha2": lambda: _sha2.sha256(b"").hexdigest().startswith("e3b0c442"),
        "_sqlite3": lambda: sqlite3.connect(":memory:").execute("select 1").fetchone() == (1,),
        "_ssl": lambda: ssl.create_default_context().verify_mode == ssl.CERT_REQUIRED,
        "binascii": lambda: binascii.crc32(b"x") == 0x8CDC1683,
        "pyexpat": lambda: xml.etree.ElementTree.fromstring("<a/>").tag == "a",
        "zlib": lambda: zlib.decompress(zlib.compress(b"x")) == b"x",
    }

    missing = set(checks) - set(modules)
    assert not missing, f"checks for modules that are not built: {sorted(missing)}"

    for name, check in checks.items():
        assert check(), f"{name} check failed"

    print(sys.version, ssl.OPENSSL_VERSION)


if __name__ == "__main__":
    main(sys.argv[1])
