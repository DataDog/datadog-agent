#!/bin/bash
# Wraps macOS `pkgbuild` so it can be invoked as a Bazel action.
#
# Usage: build_mac_pkg.sh <materialize_root.py> <root_dir> <identifier> \
#     <version> <install_location> <output> <preinstall|""> <postinstall|""> \
#     <signing_identity|"">
set -euo pipefail

MATERIALIZE_ROOT_PY="$1"
ROOT_DIR="$2"
IDENTIFIER="$3"
VERSION="$4"
INSTALL_LOCATION="$5"
OUTPUT="$6"
PREINSTALL="$7"
POSTINSTALL="$8"
SIGNING_IDENTITY="$9"

# Bazel presents action inputs (including declare_directory outputs like
# ROOT_DIR) as a symlink farm inside the sandbox, wrapping every payload
# entry in a symlink pointing at its real, ephemeral sandbox/exec-root/cache
# location. `pkgbuild` faithfully packages whatever it finds under --root,
# including symlink-ness, so a naive copy either ships broken symlinks (plain
# `cp -R`) or, if symlinks are blindly dereferenced (`cp -RL`), loses two
# things pkg_install's NativeInstaller deliberately set: exact permission
# bits (a plain `cp` applies umask to the new file's mode instead of copying
# the source mode) and intentional payload symlinks (e.g. pkg_mklink
# entries), which get flattened into duplicate regular files.
#
# materialize_root.py resolves exactly the sandbox's own indirection layer
# (dereferencing until it hits either real content or the payload's own
# intended symlink) and preserves source file modes explicitly, so it is not
# subject to the umask under which this action runs.
REAL_ROOT_DIR="$(mktemp -d)"
SCRIPTS_DIR="$(mktemp -d)"
trap 'rm -rf "$REAL_ROOT_DIR" "$SCRIPTS_DIR"' EXIT
python3 "$MATERIALIZE_ROOT_PY" "$ROOT_DIR" "$REAL_ROOT_DIR"

ARGS=(
  --root "$REAL_ROOT_DIR"
  --identifier "$IDENTIFIER"
  --version "$VERSION"
  --install-location "$INSTALL_LOCATION"
)

if [[ -n "$PREINSTALL" || -n "$POSTINSTALL" ]]; then
  if [[ -n "$PREINSTALL" ]]; then
    cp "$PREINSTALL" "$SCRIPTS_DIR/preinstall"
    chmod +x "$SCRIPTS_DIR/preinstall"
  fi
  if [[ -n "$POSTINSTALL" ]]; then
    cp "$POSTINSTALL" "$SCRIPTS_DIR/postinstall"
    chmod +x "$SCRIPTS_DIR/postinstall"
  fi

  ARGS+=(--scripts "$SCRIPTS_DIR")
fi

if [[ -n "$SIGNING_IDENTITY" ]]; then
  ARGS+=(--sign "$SIGNING_IDENTITY")
fi

mkdir -p "$(dirname "$OUTPUT")"
exec pkgbuild "${ARGS[@]}" "$OUTPUT"
