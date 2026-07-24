#!/bin/bash
# Wraps macOS `pkgbuild` so it can be invoked as a Bazel action.
#
# Usage: build_mac_pkg.sh <root_dir> <identifier> <version> <install_location> \
#     <output> <preinstall|""> <postinstall|""> <signing_identity|"">
set -euo pipefail

ROOT_DIR="$1"
IDENTIFIER="$2"
VERSION="$3"
INSTALL_LOCATION="$4"
OUTPUT="$5"
PREINSTALL="$6"
POSTINSTALL="$7"
SIGNING_IDENTITY="$8"

# Bazel presents action inputs (including declare_directory outputs like
# ROOT_DIR) as a symlink farm inside the sandbox. `pkgbuild` faithfully
# packages whatever it finds under --root, including symlink-ness, so without
# this dereferencing step every payload file would ship as a broken symlink
# pointing at an ephemeral sandbox/exec-root path instead of real content.
REAL_ROOT_DIR="$(mktemp -d)"
SCRIPTS_DIR="$(mktemp -d)"
trap 'rm -rf "$REAL_ROOT_DIR" "$SCRIPTS_DIR"' EXIT
cp -RL "$ROOT_DIR/." "$REAL_ROOT_DIR/"

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
