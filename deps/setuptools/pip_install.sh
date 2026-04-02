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
set -x

PYTHON_BIN="$1"
${PYTHON_BIN} -m pip install

exit 0

# remove Windows .exe shims on non-Windows.
find "$OUT_DIR" -name "*.exe" -delete
