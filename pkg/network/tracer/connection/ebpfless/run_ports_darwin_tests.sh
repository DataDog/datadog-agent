#!/usr/bin/env bash
# Build and/or run the ports_darwin tests for macOS (e.g. on a macOS 12 VM).
#
# Usage:
#   ./run_ports_darwin_tests.sh           # run tests in this directory (darwin only)
#   ./run_ports_darwin_tests.sh -c         # build test binary only (for copying to VM)
#   ./run_ports_darwin_tests.sh -r binary  # run a pre-built binary (e.g. on VM)
#
# On a macOS 12 VM you can either:
#   1. Clone the repo on the VM and run: go test -v ./pkg/network/tracer/connection/ebpfless/
#   2. Build the binary on another Mac (same arch): go test -c -o ebpfless.test ./pkg/network/tracer/connection/ebpfless/
#      Copy ebpfless.test to the VM and run: ./ebpfless.test -test.v
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/../../../../.."  # repo root

case "${1:-}" in
  -c)
    echo "Building test binary (darwin only)..."
    go test -c -o ebpfless.test ./pkg/network/tracer/connection/ebpfless/
    echo "Created: $(pwd)/ebpfless.test"
    echo "Copy this binary to your macOS 12 VM and run: ./ebpfless.test -test.v"
    ;;
  -r)
    if [[ -n "${2:-}" && -x "$2" ]]; then
      exec "$2" -test.v
    else
      if [[ -x "./ebpfless.test" ]]; then
        exec ./ebpfless.test -test.v
      else
        echo "Usage: $0 -r [path/to/ebpfless.test]" 1>&2
        exit 1
      fi
    fi
    ;;
  *)
    echo "Running ports_darwin tests..."
    go test -v ./pkg/network/tracer/connection/ebpfless/
    ;;
esac
