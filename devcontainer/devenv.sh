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
DOCKERFILE="${REPO_ROOT}/devcontainer/Dockerfile"

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

# Always drop into the bits user after the entrypoint's setup runs.
CMD=(gosu bits bash)
if [[ $# -gt 0 ]]; then
    CMD=(gosu bits "$@")
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

# Core mounts: repo, build caches, and home.
# datadog-agent-bits-home seeds from the image on first use and persists shell history,
# entrypoint-generated configs, and any container-side state across runs.
# datadog-agent-dd-cache covers GOPATH/GOCACHE/BUNDLE_PATH (all under /var/cache/dd in
# this image), kept separate so it can be wiped independently of the home volume.
MOUNTS+=(-v "${REPO_ROOT}:/workspace:cached")
MOUNTS+=(-v "datadog-agent-bits-home:/home/bits")
MOUNTS+=(-v "datadog-agent-dd-cache:/var/cache/dd")
MOUNTS+=(-v "/var/run/docker.sock:/var/run/docker.sock")

# Host credential/config overlays — bind-mounted on top of the home volume so the container
# always uses the host's current credentials rather than a stale snapshot in the volume.
# Docker applies mounts depth-first, so these win over the named volume at /home/bits.

# claude: managed auth settings (system path) + user state + root config file
add_mount "${CLAUDE_BIN}"                          /usr/local/bin/claude                ro
add_mount /etc/claude-code                         /etc/claude-code                     ro
add_mount "${HOME}/.claude.json"                   /home/bits/.claude.json
add_mount "${HOME}/.claude"                        /home/bits/.claude

# gh: token lives in hosts.yml and is rewritten on refresh — mount rw
add_mount "${HOME}/.config/gh"                     /home/bits/.config/gh

# git: config, signing program, and commit hooks
add_mount "${HOME}/.gitconfig"                     /home/bits/.gitconfig                ro
add_mount "${HOME}/.config/gitsign"                /home/bits/.config/gitsign           ro
add_mount "${HOME}/.global_hooks"                  /home/bits/.global_hooks             ro
add_mount "${HOME}/.tmux.conf"                     /home/bits/.tmux.conf                ro

# SSH: keys read-only; live agent socket for operations
add_mount "${HOME}/.ssh"                           /home/bits/.ssh                      ro
add_mount "${SSH_AGENT_SOCK}"                      /ssh-agent

# GPG / pass: pass uses ~/.config/password-store/gpg (not ~/.gnupg) as its GPG homedir.
# Both dirs are rw so the GPG agent can write state back through the bind mounts.
add_mount "${HOME}/.gnupg"                         /home/bits/.gnupg
add_mount "${HOME}/.config/password-store/gpg"     /home/bits/.config/password-store/gpg
add_mount "${HOME}/.password-store"                /home/bits/.password-store           ro

ENV_FLAGS=(
    # Map the host user's UID/GID into the container so file ownership is correct.
    # The entrypoint creates the bits user with these IDs on first boot.
    -e HOST_UID="$(id -u)"
    -e HOST_GID="$(id -g)"
    -e SSH_AUTH_SOCK=/ssh-agent
    # Tell pass where its dedicated GPG homedir is (set on the host via /etc/profile.d/).
    -e PASSWORD_STORE_GPG_OPTS="--homedir /home/bits/.config/password-store/gpg"
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
