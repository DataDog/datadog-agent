#!/bin/bash
# Fails unless the given binary is statically linked, mirroring the check
# `tasks/dogstatsd.py:size_test` performs for the omnibus-built binary.
set -euo pipefail

bin="$(readlink -f "$1")"

if ! file "$bin" | grep -q "statically linked"; then
  echo "dogstatsd binary is not statically linked:"
  file "$bin"
  exit 1
fi

echo "dogstatsd binary is statically linked"
