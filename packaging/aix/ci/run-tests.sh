#!/bin/sh

# Run datadog-agent Go unit tests on an AIX/ppc64 host.
#
# Invoked over SSH by the CI job.
# Assumes that packaging/aix/ci/setup-host.sh has already been run on this host.

set -eu

# AIX Toolbox installs under /opt/freeware/bin, which the default root PATH
# omits. Export it before sourcing env.sh, which runs `invoke agent.version`
# for AGENT_VERSION detection before setting its own PATH.
PATH=/opt/freeware/bin:/usr/bin:/etc:/usr/sbin:/usr/ucb:/usr/bin/X11:/sbin
export PATH

# Source the shared AIX build environment (PATH, GOROOT, CC/CXX, CGO flags,
# GOTOOLCHAIN=local, GOCACHE, TMPDIR, ...). env.sh resolves AGENT_SRC by walking
# up from $0 to the nearest .git ancestor. It pins CC/CXX -> gcc-8, which works
# on AIX 7.3 (gcc-8's include-fixed headers match the 7.3 system headers).
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

# AIX's default latin-1 locale crashes invoke's stdout handler on gotestsum's
# unicode (✖/✓). Force UTF-8.
export PYTHONIOENCODING=utf-8
export LC_ALL=en_US.UTF-8

cd "$AGENT_SRC"

# --only-modified-packages: test only packages with changed Go files (uses
#   `git merge-base HEAD origin/main`; needs a full clone — see
#   checkout-source.sh).
# --build-exclude=python: rtloader / embedded Python / Rust extensions are not
#   provisioned on this host (Go-only unit tests).
# --build-cpus=1: SiteOX LPARs are memory-limited (a single Go compile uses
#   3-4 GiB); serial builds avoid OOM on cgo-heavy packages.
python3.12 -m invoke -e test \
    --only-modified-packages \
    --build-exclude=python \
    --build-cpus=1
