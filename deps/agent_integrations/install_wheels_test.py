# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

import base64
import csv
import hashlib
import io
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock

import install_wheels

DIST_INFO = "psycopg_c-3.2.6.dist-info"
EXCLUDED_FILES = {
    "psycopg_c/_psycopg.c",
    "psycopg_c/pq.c",
}
WHEEL_FILES = {
    "psycopg_c/__init__.py": b"",
    "psycopg_c/_psycopg.c": b"/* generated extension source */\n",
    "psycopg_c/pq.c": b"/* generated libpq source */\n",
    "psycopg_c/_psycopg.cpython-312-x86_64-linux-gnu.so": b"compiled _psycopg placeholder\n",
    "psycopg_c/pq.cpython-312-x86_64-linux-gnu.so": b"compiled pq placeholder\n",
    "psycopg_c/_psycopg.pyi": b"def connect() -> None: ...\n",
    "psycopg_c/pq.pxd": b"cdef int libpq_version\n",
    "psycopg_c/types/numutils.c": b"/* generated numeric helper source */\n",
    "another_package/native.c": b"/* unrelated package source */\n",
    f"{DIST_INFO}/METADATA": b"Metadata-Version: 2.1\nName: psycopg-c\nVersion: 3.2.6\n",
    f"{DIST_INFO}/WHEEL": (
        b"Wheel-Version: 1.0\n"
        b"Generator: install-wheels-test\n"
        b"Root-Is-Purelib: false\n"
        b"Tag: cp312-cp312-manylinux_2_17_x86_64\n"
    ),
}
RECORD_PATH = f"{DIST_INFO}/RECORD"


def _record_contents(files: dict[str, bytes]) -> bytes:
    output = io.StringIO(newline="")
    writer = csv.writer(output, lineterminator="\n")
    for path, contents in files.items():
        digest = base64.urlsafe_b64encode(hashlib.sha256(contents).digest()).rstrip(b"=").decode()
        writer.writerow((path, f"sha256={digest}", len(contents)))
    writer.writerow((RECORD_PATH, "", ""))
    return output.getvalue().encode()


def _build_wheel(path: Path) -> None:
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as wheel:
        for filename, contents in WHEEL_FILES.items():
            wheel.writestr(filename, contents)
        wheel.writestr(RECORD_PATH, _record_contents(WHEEL_FILES))


class InstallWheelsTest(unittest.TestCase):
    def test_excludes_only_exact_configured_wheel_files_and_regenerates_record(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            wheel = root / "psycopg_c-3.2.6-cp312-cp312-manylinux_2_17_x86_64.whl"
            runtime_output = root / "runtime"
            bin_output = root / "bin"
            _build_wheel(wheel)
            with install_wheels.WheelFile.open(wheel) as source:
                source.validate_record()

            argv = [
                "install_wheels.py",
                "--runtime-output",
                str(runtime_output),
                "--bin-output",
                str(bin_output),
                "--python-version",
                "3.12",
                "--interpreter",
                "/opt/datadog-agent/embedded/bin/python3",
                "--entrypoints-dirname",
                "bin",
                "--platform",
                "posix",
            ]
            for excluded_file in sorted(EXCLUDED_FILES):
                argv.extend(("--exclude", excluded_file))
            argv.append(str(wheel))

            with mock.patch.object(sys, "argv", argv):
                install_wheels.main()

            site_packages = runtime_output / "lib/python3.12/site-packages"
            installed_files = {
                path.relative_to(site_packages).as_posix() for path in site_packages.rglob("*") if path.is_file()
            }
            expected_files = (set(WHEEL_FILES) | {RECORD_PATH}) - EXCLUDED_FILES
            self.assertEqual(installed_files, expected_files)

            with (site_packages / RECORD_PATH).open(newline="") as record_file:
                recorded_files = {row[0] for row in csv.reader(record_file)}
            self.assertEqual(recorded_files, expected_files)


if __name__ == "__main__":
    unittest.main()
