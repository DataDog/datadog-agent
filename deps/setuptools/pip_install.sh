#!/usr/bin/env bash
# Install setuptools into a target directory using the cpython runtime we built.
#
# Arguments:
#   $1  PYTHON_BIN       — path to the python3.x binary
#   $2  PYTHON_STDLIB    — path to the stdlib directory (copy_to_directory output "python3.x")
#   $3  PYTHON_LIB       — path to libpython3.x.so / libpython3.x.dylib
#   $4  PYPROJECT_TOML   — path to setuptools/pyproject.toml (dirname = source root)
#   $5  OUT_DIR          — directory to install into (pip --target)

set -euo pipefail

PYTHON_BIN="$1"
PYTHON_STDLIB="$2"
PYTHON_LIB="$3"
PYPROJECT_TOML="$4"
OUT_DIR="$5"

SRC="$(cd "$(dirname "$PYPROJECT_TOML")" && pwd)"

# Build a PYTHONHOME so Python can find its stdlib.
# python_lib_dir_unix is a directory named "python3.x" containing the stdlib
# files directly.  PYTHONHOME must be a directory with lib/python3.x/ under it.
TMPENV="$(mktemp -d)"
trap 'rm -rf "$TMPENV"' EXIT
mkdir -p "$TMPENV/lib"
ln -sf "$(cd "$PYTHON_STDLIB" && pwd)" "$TMPENV/lib/$(basename "$PYTHON_STDLIB")"
export PYTHONHOME="$TMPENV"

# Make libpython findable at link time.
LIBDIR="$(cd "$(dirname "$PYTHON_LIB")" && pwd)"
export LD_LIBRARY_PATH="${LIBDIR}${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export DYLD_LIBRARY_PATH="${LIBDIR}${DYLD_LIBRARY_PATH:+:$DYLD_LIBRARY_PATH}"

# Bootstrap: add the setuptools source tree to PYTHONPATH so pip can import
# setuptools as its own PEP 517 build backend without hitting PyPI.
# (Python 3.12+ no longer bundles setuptools in ensurepip, so without this
# trick `pip install .` would try to download the build backend.)
export PYTHONPATH="$SRC${PYTHONPATH:+:$PYTHONPATH}"

mkdir -p "$OUT_DIR"

"$PYTHON_BIN" -m pip install \
    --no-build-isolation \
    --no-deps \
    --target="$OUT_DIR" \
    "$SRC"

# Match omnibus setuptools3.rb: remove Windows .exe shims on non-Windows.
find "$OUT_DIR" -name "*.exe" -delete
