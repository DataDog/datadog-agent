#!/usr/bin/env bash
# Start an interactive shell inside the datadog-agent CI build environment.
# The repo is mounted at /workspace; Go module and build caches are persisted
# in named Docker volumes so incremental builds are fast.
#
# Usage:
#   ./dev/devenv.sh              # interactive bash shell
#   ./dev/devenv.sh --rebuild    # force rebuild of the wrapper image, then shell
#   ./dev/devenv.sh <cmd>...     # run a one-shot command, e.g. dda inv dogstatsd.build
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="datadog-agent-devenv:local"
DOCKERFILE="${REPO_ROOT}/dev/Dockerfile.devenv"

build_image() {
    echo "Building devenv wrapper image..."
    docker build -t "${IMAGE}" - < "${DOCKERFILE}"
}

if ! docker image inspect "${IMAGE}" >/dev/null 2>&1; then
    build_image
fi

if [[ "${1:-}" == "--rebuild" ]]; then
    build_image
    shift
fi

CMD=(bash)
if [[ $# -gt 0 ]]; then
    CMD=("$@")
fi

# Resolve paths that can't be evaluated as literals inside docker run flags.
# claude is a dotslash wrapper that resolves to a versioned binary under .local/share.
CLAUDE_BIN="$(readlink -f "$(command -v claude)" 2>/dev/null || true)"
# The SSH agent socket path rotates on agent restart; resolve the symlink now.
SSH_AGENT_SOCK="$(readlink -f "${HOME}/.ssh/ssh_auth_sock" 2>/dev/null || true)"

# Build the mount list. Each add_mount call appends only if the source path exists,
# so the script degrades gracefully on machines missing a given credential.
MOUNTS=()

add_mount() {
    local src="$1" dst="$2" opts="${3:-}"
    if [[ -e "$src" ]]; then
        if [[ -n "$opts" ]]; then
            MOUNTS+=(-v "${src}:${dst}:${opts}")
        else
            MOUNTS+=(-v "${src}:${dst}")
        fi
    fi
}

# Core repo + persistent home (Go module cache, dda cache, go build cache all live here)
MOUNTS+=(-v "${REPO_ROOT}:/workspace:cached")
MOUNTS+=(-v "datadog-agent-bits-home:/home/bits")
MOUNTS+=(-v "/var/run/docker.sock:/var/run/docker.sock")

# Statically linked binaries — bind-mounted so the container tracks the host version
# without requiring an image rebuild when these tools are upgraded.
add_mount /usr/bin/gh                         /usr/local/bin/gh          ro
add_mount "${CLAUDE_BIN}"                     /usr/local/bin/claude      ro
add_mount "${HOME}/.local/bin/ddtool"         /usr/local/bin/ddtool      ro
add_mount "${HOME}/.local/bin/dd-gitsign"     /usr/local/bin/dd-gitsign  ro

# claude: managed auth settings (system path, no home-volume conflict) + user state
add_mount /etc/claude-code                    /etc/claude-code           ro
add_mount "${HOME}/.claude"                   /home/bits/.claude

# gh: token lives in hosts.yml and is rewritten on refresh — mount rw
add_mount "${HOME}/.config/gh"                /home/bits/.config/gh

# git: config, signing program inputs, and commit hooks
add_mount "${HOME}/.gitconfig"                /home/bits/.gitconfig      ro
add_mount "${HOME}/.config/gitsign"           /home/bits/.config/gitsign ro
add_mount "${HOME}/.global_hooks"             /home/bits/.global_hooks   ro
add_mount "${HOME}/.tmux.conf"                /home/bits/.tmux.conf      ro

# SSH: known_hosts + authorized_keys read-only; live agent socket for key operations
add_mount "${HOME}/.ssh"                      /home/bits/.ssh            ro
add_mount "${SSH_AGENT_SOCK}"                 /ssh-agent

# GPG / pass: required for the ddtool → Vault → GPG auth chain that backs claude + dd-gitsign.
# .gnupg is rw because gpg-agent writes state; .password-store is read-only.
add_mount "${HOME}/.gnupg"                    /home/bits/.gnupg
add_mount "${HOME}/.password-store"           /home/bits/.password-store ro

ENV_FLAGS=(
    -e SSH_AUTH_SOCK=/ssh-agent
    # Tell pass/aws-vault where to find the credential store.
    -e AWS_VAULT_BACKEND=pass
    -e AWS_VAULT_PASS_PREFIX=aws-vault
)

exec docker run -it --rm \
    "${MOUNTS[@]}" \
    "${ENV_FLAGS[@]}" \
    -w /workspace \
    "${IMAGE}" \
    "${CMD[@]}"
