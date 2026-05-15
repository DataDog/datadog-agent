#!/usr/bin/env bash
# Build libpatterns.so for linux_amd64 and linux_arm64 inside a Debian Bullseye
# Docker container (glibc 2.31), safely below the 2.33/2.34 symbols that break
# the omnibus cross-compiler. Uses the official rust:slim-bullseye image so Rust
# is pre-installed and no rustup download is needed.
#
# Usage:
#   ./scripts/build_libpatterns.sh [--dd-source PATH] [--arch amd64|arm64|both]
#
# Defaults:
#   --dd-source  ~/dd/dd-source
#   --arch       both

set -euo pipefail

DD_SOURCE_PATH="${DD_SOURCE:-$HOME/dd/dd-source}"
ARCH="both"

while [[ $# -gt 0 ]]; do
  case $1 in
    --dd-source) DD_SOURCE_PATH="$2"; shift 2 ;;
    --arch)      ARCH="$2";           shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

PATTERNS_DIR="domains/data_science/libs/rust/patterns"
PATTERNS_PATH="$DD_SOURCE_PATH/$PATTERNS_DIR"
AGENT_VENDOR="$(cd "$(dirname "$0")/.." && pwd)/pkg/logs/patterns/tokenizer/rust/vendor"

if [[ ! -d "$PATTERNS_PATH" ]]; then
  echo "ERROR: patterns library not found at $PATTERNS_PATH" >&2
  echo "Set --dd-source or the DD_SOURCE env var to your dd-source checkout." >&2
  exit 1
fi

# Standalone Cargo.toml — replaces workspace references and adds cdylib target.
# Versions must match the workspace Cargo.toml in dd-source.
STANDALONE_CARGO_TOML='[package]
name = "patterns"
version = "0.1.0"
edition = "2024"

[lib]
crate-type = ["cdylib"]

[dependencies]
regex-automata = { version = "0.4", features = ["dfa-build", "dfa-search"] }
thiserror = "1.0.69"
once_cell = "1.19"
flatbuffers = "24.3"
'

build_arch() {
  local arch="$1"          # amd64 | arm64
  local vendor_dir

  case "$arch" in
    amd64) vendor_dir="$AGENT_VENDOR/linux_amd64" ;;
    arm64) vendor_dir="$AGENT_VENDOR/linux_arm64" ;;
    *)     echo "Unknown arch: $arch" >&2; exit 1 ;;
  esac

  echo ""
  echo "=== Building libpatterns.so for linux/$arch (glibc 2.31 / Debian Bullseye) ==="

  docker run --rm \
    --platform "linux/$arch" \
    -v "$PATTERNS_PATH:/src:ro" \
    -v "$AGENT_VENDOR:/out" \
    rust:slim-bullseye \
    bash -c "
      set -euo pipefail

      # Copy source to a writable directory and patch Cargo.toml
      cp -r /src /build
      chmod -R u+w /build

      cat > /build/Cargo.toml << 'TOML'
$STANDALONE_CARGO_TOML
TOML

      cd /build
      cargo build --release

      cp target/release/libpatterns.so /out/linux_${arch}/libpatterns.so
      echo 'Done: /out/linux_${arch}/libpatterns.so'
    "

  echo "Vendored: $vendor_dir/libpatterns.so"
}

case "$ARCH" in
  amd64) build_arch amd64 ;;
  arm64) build_arch arm64 ;;
  both)  build_arch amd64; build_arch arm64 ;;
  *)     echo "Unknown --arch value: $ARCH (use amd64, arm64, or both)" >&2; exit 1 ;;
esac

echo ""
echo "All done. Verify glibc requirements with:"
echo "  objdump -p $AGENT_VENDOR/linux_amd64/libpatterns.so | grep GLIBC"
