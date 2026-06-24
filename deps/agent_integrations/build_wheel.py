#!/usr/bin/env python3
"""Build a wheel from source using `build`.

The choice of `build` as the build frontend is based on it being the most minimalist
officially supported PEP-517-compliant build frontend, with a very minimal set of dependencies
and with reduced risk of side effects. Using `pip` would potentially require more care
around controlling the environment to ensure the right build dependencies (like a predefined build backend)
are present and we don't break hermeticity.
"""

import argparse
import glob
import os
import zipfile

from build import ProjectBuilder
from hatchling.utils.fs import locate_file

PHP_FPM_VENDOR_FILES = (
    "datadog_checks/php_fpm/vendor/__init__.py",
    "datadog_checks/php_fpm/vendor/fcgi_app.py",
    "datadog_checks/php_fpm/vendor/fcgi_app_py2.py",
)


def _is_php_fpm_source(src):
    return os.path.basename(os.path.abspath(src)) == "php_fpm"


def _print_relevant_gitignore_lines(gitignore):
    print(f"--- begin relevant lines from {gitignore} ---", flush=True)
    with open(gitignore, encoding="utf-8") as f:
        for line in f:
            stripped = line.strip()
            if "vendor" in stripped or stripped in {"build/", "dist/"}:
                print(line.rstrip(), flush=True)
    print(f"--- end relevant lines from {gitignore} ---", flush=True)


def _print_gitignore_probe(src):
    src_abs = os.path.abspath(src)
    src_real = os.path.realpath(src_abs)

    print("=== DD DEBUG php_fpm wheel gitignore probe ===", flush=True)
    print(f"cwd={os.getcwd()}", flush=True)
    print(f"src={src}", flush=True)
    print(f"src_abs={src_abs}", flush=True)
    print(f"src_real={src_real}", flush=True)

    for label, root in (("abs", src_abs), ("real", src_real)):
        gitignore = locate_file(root, ".gitignore")
        print(f"{label} locate_file(.gitignore)={gitignore}", flush=True)
        if gitignore and os.path.isfile(gitignore):
            _print_relevant_gitignore_lines(gitignore)

    for relpath in PHP_FPM_VENDOR_FILES:
        path = os.path.join(src_abs, relpath)
        print(f"source {relpath}: exists={os.path.exists(path)}", flush=True)


def _print_wheel_probe(output_dir):
    wheels = sorted(glob.glob(os.path.join(output_dir, "*.whl")))
    print(f"wheels={wheels}", flush=True)

    for wheel in wheels:
        print(f"--- inspecting wheel {wheel} ---", flush=True)
        with zipfile.ZipFile(wheel) as zf:
            names = zf.namelist()
            vendor_names = [name for name in names if "datadog_checks/php_fpm/vendor" in name]
            print(f"vendor entries={vendor_names}", flush=True)

            for record_name in sorted(name for name in names if name.endswith(".dist-info/RECORD")):
                info = zf.getinfo(record_name)
                print(f"{record_name} size={info.file_size}", flush=True)
                record = zf.read(record_name).decode()
                for line in record.splitlines():
                    if "php_fpm/vendor" in line:
                        print(f"RECORD vendor line: {line}", flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--src", required=True)
    parser.add_argument("--output-dir", required=True)
    args = parser.parse_args()

    is_php_fpm = _is_php_fpm_source(args.src)
    if is_php_fpm:
        _print_gitignore_probe(args.src)

    ProjectBuilder(args.src).build("wheel", args.output_dir)

    if is_php_fpm:
        _print_wheel_probe(args.output_dir)
        raise SystemExit("intentional php_fpm wheel debug failure")


if __name__ == "__main__":
    main()
