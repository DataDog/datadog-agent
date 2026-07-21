#!/usr/bin/env bash

set -euo pipefail

usage() {
    cat <<'EOF'
Usage: tools/build-privileged-rshell-agent.sh [OUTPUT_DIRECTORY]

Build the Datadog Agent, Private Action Runner, and privileged rshell helper
for the current Linux machine using dda. The default output directory is:

    bin/privileged-rshell-bundle
EOF
}

if [[ ${1:-} == "-h" || ${1:-} == "--help" ]]; then
    usage
    exit 0
fi
if (( $# > 1 )); then
    usage >&2
    exit 2
fi
if [[ $(uname -s) != "Linux" ]]; then
    echo "error: the privileged rshell helper is supported only on Linux" >&2
    exit 1
fi
if ! command -v dda >/dev/null 2>&1; then
    echo "error: dda is required; install the Datadog developer tooling first" >&2
    exit 1
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
output_dir=${1:-"$repo_root/bin/privileged-rshell-bundle"}

cd -- "$repo_root"

# These tasks select the repository's normal build tags and compile for the
# current host. rshell.build additionally forces CGO_ENABLED=0 because the
# helper requires Go's all-runtime-thread credential transition on Linux.
dda inv agent.build
dda inv privateactionrunner.build
dda inv rshell.build

install -d -m 0755 "$output_dir/bin" "$output_dir/systemd"
install -m 0755 bin/agent/agent "$output_dir/bin/datadog-agent"
install -m 0755 bin/privateactionrunner/privateactionrunner "$output_dir/bin/private-action-runner"
install -m 0755 bin/rshell/rshell "$output_dir/bin/rshell"
install -m 0644 \
    pkg/fleet/installer/packages/embedded/tmpl/gen/debrpm/datadog-agent-rshell-privileged.service \
    "$output_dir/systemd/datadog-agent-rshell-privileged.service"
install -m 0644 \
    pkg/fleet/installer/packages/embedded/tmpl/gen/debrpm/datadog-agent-rshell-privileged.socket \
    "$output_dir/systemd/datadog-agent-rshell-privileged.socket"

echo "Built native privileged-rshell Agent bundle at: $output_dir"
