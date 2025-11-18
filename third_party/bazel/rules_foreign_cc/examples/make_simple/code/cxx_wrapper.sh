#!/usr/bin/env bash

set -o errexit
set -o nounset

if [[ $(uname) == *"NT"* ]]; then
 # If Windows
  exec clang-cl "$@"
else
  exec "$CXX" "$@"
fi
