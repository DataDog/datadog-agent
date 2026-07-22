#!/usr/bin/env bash

set -euo pipefail

usage() {
    cat <<'EOF'
Usage: tools/build-privileged-rshell-agent.sh [--target TARGET_TRIPLE] [OUTPUT_DIRECTORY]

Build the Datadog Agent, Private Action Runner, and privileged rshell helper
for the current Linux machine using dda. Supported target triples are:

    x86_64-unknown-linux-gnu
    x86_64-unknown-linux-musl
    aarch64-unknown-linux-gnu

The target must match the current machine because the Agent build includes
native cgo and Bazel-built rtloader artifacts. When --target is omitted, it is
detected from the machine. The default output directory is:

    bin/privileged-rshell-bundle

When --target is specified, its triple is appended to the default directory.

If dda or Bazelisk is unavailable, the script bootstraps them into a temporary
directory for the duration of the build. No tools are installed system-wide.
EOF
}

bootstrap_root=""

cleanup() {
    if [[ -n $bootstrap_root ]]; then
        rm -rf -- "$bootstrap_root"
    fi
}

ensure_bootstrap_root() {
    if [[ -z $bootstrap_root ]]; then
        bootstrap_root=$(mktemp -d "${TMPDIR:-/tmp}/privileged-rshell-build.XXXXXXXX")
        install -d -m 0755 "$bootstrap_root/bin"
    fi
}

download() {
    local url=$1
    local destination=$2

    if command -v curl >/dev/null 2>&1; then
        curl --proto '=https' --tlsv1.2 -fsSL --retry 4 --output "$destination" "$url"
    elif command -v wget >/dev/null 2>&1; then
        if [[ $url != https://* ]]; then
            echo "error: refusing to download dda over a non-HTTPS URL" >&2
            exit 1
        fi
        # Use the common BusyBox/GNU option forms so this also works on musl
        # distributions where wget may be provided by BusyBox.
        wget -q -t 4 -O "$destination" "$url"
    else
        echo "error: curl or wget is required to download dda" >&2
        exit 1
    fi
}

uses_musl() {
    [[ -f /etc/alpine-release ]] || { command -v ldd >/dev/null 2>&1 && ldd --version 2>&1 | grep -qi musl; }
}

detect_host_target() {
    local architecture
    local libc

    architecture=$(uname -m)
    case $architecture in
        x86_64 | amd64) architecture=x86_64 ;;
        aarch64 | arm64) architecture=aarch64 ;;
        *)
            echo "error: the Agent bundle does not support Linux architecture: $architecture" >&2
            exit 1
            ;;
    esac

    if uses_musl; then
        libc=musl
    else
        libc=gnu
    fi
    printf '%s-unknown-linux-%s\n' "$architecture" "$libc"
}

configure_target() {
    case $target_triple in
        x86_64-unknown-linux-gnu)
            target_goarch=amd64
            target_uses_glibc=true
            ;;
        x86_64-unknown-linux-musl)
            target_goarch=amd64
            target_uses_glibc=false
            ;;
        aarch64-unknown-linux-gnu)
            target_goarch=arm64
            target_uses_glibc=true
            ;;
        *)
            echo "error: unsupported target triple: $target_triple" >&2
            echo "supported targets: x86_64-unknown-linux-gnu, x86_64-unknown-linux-musl, aarch64-unknown-linux-gnu" >&2
            exit 1
            ;;
    esac
}

bootstrap_dda() {
    local architecture
    local asset
    local archive

    architecture=$(uname -m)
    case $architecture in
        x86_64 | amd64)
            if uses_musl; then
                asset=dda-x86_64-unknown-linux-musl.tar.gz
            else
                asset=dda-x86_64-unknown-linux-gnu.tar.gz
            fi
            ;;
        aarch64 | arm64)
            if uses_musl; then
                echo "error: dda does not publish an ARM64 musl release" >&2
                exit 1
            fi
            asset=dda-aarch64-unknown-linux-gnu.tar.gz
            ;;
        ppc64le)
            if uses_musl; then
                echo "error: dda does not publish a PowerPC64LE musl release" >&2
                exit 1
            fi
            asset=dda-powerpc64le-unknown-linux-gnu.tar.gz
            ;;
        *)
            echo "error: dda does not publish a Linux release for architecture: $architecture" >&2
            exit 1
            ;;
    esac

    ensure_bootstrap_root
    archive="$bootstrap_root/$asset"
    echo "dda not found; downloading $asset" >&2
    download "https://github.com/DataDog/datadog-agent-dev/releases/latest/download/$asset" "$archive"

    # Release archives contain one top-level executable. Extract only that
    # member so an unexpected archive cannot place other files on the host.
    tar -xzf "$archive" -C "$bootstrap_root/bin" dda
    chmod 0755 "$bootstrap_root/bin/dda"
    dda_path="$bootstrap_root/bin/dda"
}

dda_inv() {
    local command_path=$PATH

    if [[ -n $bootstrap_root ]]; then
        command_path="$bootstrap_root/bin:$command_path"
    fi
    # Standalone dda resolves the invoke dependencies on first use. Explicitly
    # allow that even on hosts which inherited the build-image opt-out setting.
    DDA_INTERACTIVE=false DDA_NO_DYNAMIC_DEPS=0 GOARCH="$target_goarch" GOOS=linux PATH="$command_path" \
        "$dda_path" inv "$@"
}

trap cleanup EXIT

target_triple=""
target_was_explicit=false
output_argument=""
while (( $# > 0 )); do
    case $1 in
        -h | --help)
            usage
            exit 0
            ;;
        --target)
            if (( $# < 2 )) || [[ -z $2 ]]; then
                echo "error: --target requires a target triple" >&2
                usage >&2
                exit 2
            fi
            target_triple=$2
            target_was_explicit=true
            shift 2
            ;;
        --target=*)
            target_triple=${1#*=}
            if [[ -z $target_triple ]]; then
                echo "error: --target requires a target triple" >&2
                usage >&2
                exit 2
            fi
            target_was_explicit=true
            shift
            ;;
        --)
            shift
            if (( $# > 1 )); then
                echo "error: expected at most one output directory" >&2
                usage >&2
                exit 2
            fi
            output_argument=${1:-}
            break
            ;;
        -*)
            echo "error: unknown option: $1" >&2
            usage >&2
            exit 2
            ;;
        *)
            if [[ -n $output_argument ]]; then
                echo "error: expected at most one output directory" >&2
                usage >&2
                exit 2
            fi
            output_argument=$1
            shift
            ;;
    esac
done

if [[ $(uname -s) != "Linux" ]]; then
    echo "error: the privileged rshell helper is supported only on Linux" >&2
    exit 1
fi
if ! command -v tar >/dev/null 2>&1; then
    echo "error: tar is required to unpack the dda release" >&2
    exit 1
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
host_target=$(detect_host_target)
if [[ -z $target_triple ]]; then
    target_triple=$host_target
fi
configure_target

if [[ $target_triple != "$host_target" ]]; then
    echo "error: target $target_triple does not match this build host ($host_target)" >&2
    echo "cross-compilation is unsafe here because the Agent links native cgo and rtloader artifacts" >&2
    exit 1
fi

if [[ -n $output_argument ]]; then
    output_dir=$output_argument
elif $target_was_explicit; then
    output_dir="$repo_root/bin/privileged-rshell-bundle/$target_triple"
else
    output_dir="$repo_root/bin/privileged-rshell-bundle"
fi

cd -- "$repo_root"

if command -v dda >/dev/null 2>&1; then
    dda_path=$(command -v dda)
else
    bootstrap_dda
fi

# The default Agent build uses Bazel for rtloader. install-tools installs the
# repository-pinned Bazelisk and creates its bazel alias; GOBIN keeps all of
# those tools local to this build instead of modifying the target system.
if ! command -v bazelisk >/dev/null 2>&1; then
    if ! command -v go >/dev/null 2>&1; then
        echo "error: Go is required to install the Agent's pinned build tools" >&2
        exit 1
    fi
    ensure_bootstrap_root
    echo "bazelisk not found; installing the Agent's pinned Go tools" >&2
    GOBIN="$bootstrap_root/bin" dda_inv install-tools
fi

# These tasks select the repository's normal build tags and compile for the
# current host. rshell.build additionally forces CGO_ENABLED=0 because the
# helper requires Go's all-runtime-thread credential transition on Linux.
agent_build_args=()
if ! $target_uses_glibc; then
    agent_build_args+=(--no-glibc)
fi
dda_inv agent.build "${agent_build_args[@]}"
dda_inv privateactionrunner.build
dda_inv rshell.build

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

echo "Built native $target_triple privileged-rshell Agent bundle at: $output_dir"
