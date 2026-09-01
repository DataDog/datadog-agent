#!/bin/sh

# Run datadog-agent Go unit tests on an AIX/ppc64 host.
#
# Invoked over SSH by the CI job. Assumes setup-host.sh has already been run.

set -eu

# Source the shared AIX build environment (PATH, GOROOT, CC/CXX, CGO flags,
# GOTOOLCHAIN=local, GOCACHE, TMPDIR, ...). env.sh resolves AGENT_SRC by
# walking up from $0 to the nearest .git ancestor.
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

cd "$AGENT_SRC"

# --only-modified-packages: test only packages with changed Go files.
# --build-exclude=python: rtloader/embedded Python/Rust aren't provisioned
#   on this host (Go-only unit tests).
# --build-cpus=1: serial builds avoid OOM on memory-limited hosts.
python3.12 -m invoke -e test \
    --only-modified-packages \
    --build-exclude=python \
    --build-cpus=1
