#!/usr/bin/env bash
# Bazel workspace status script.
#
# Provides stable variables consumed by rules_go x_defs stamping, e.g.
#   x_defs = {"<importpath>.GitCommit": "{STABLE_GIT_COMMIT}"}
# Values are substituted at link time, but only when the build runs with --stamp.
# See tasks/new_e2e_tests.py (_build_single_binary) for the e2e prebuilt-binaries flow
# that relies on this, and bazel/rules/go/go_binary.bzl for the pending repo-wide usage.
set -euo pipefail

echo "STABLE_GIT_COMMIT $(git rev-parse --short HEAD)"
