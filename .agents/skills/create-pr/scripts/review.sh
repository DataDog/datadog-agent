#!/usr/bin/env bash
# Run the local AI code review for /create-pr with the first available reviewer:
#   1. codex on the host, the same provider as the PR `Code review` workflow
#   2. claude on the host
#   3. claude in a `dda env dev` developer environment, which authenticates through `ddtool`
# If a review fails, the next reviewer is tried. Extra arguments are passed to `dda review`.
#
# Exit codes: 0 a review completed, 1 every attempted review failed, 2 no reviewer is available.
set -uo pipefail

log() { echo "create-pr review: $*" >&2; }

if ! command -v dda >/dev/null; then
    log "dda is not installed, see https://datadoghq.dev/datadog-agent-dev/"
    exit 2
fi

repo_root=$(git rev-parse --show-toplevel) || exit 1
cd "$repo_root" || exit 1

attempted=0

run_review() {
    local reviewer=$1
    shift
    attempted=1
    log "reviewing with ${reviewer}"
    if "$@"; then
        exit 0
    fi
    log "the review with ${reviewer} failed"
}

# 1. codex, which matches the reviewer used in CI
if ! command -v codex >/dev/null; then
    log "codex is not installed"
elif ! codex login status >/dev/null 2>&1; then
    log "codex is installed but not logged in, run \`codex login\`"
else
    run_review "codex (same reviewer as CI)" dda review "$@"
fi

# 2. claude on the host
if ! command -v claude >/dev/null; then
    log "claude is not installed"
elif ! claude auth status >/dev/null 2>&1; then
    log "claude is installed but not logged in, run \`claude auth login\`"
else
    run_review "claude (CI uses codex, so findings may differ)" dda review --provider claude "$@"
fi

# 3. claude in a developer environment
discard_dev_env() {
    log "removing developer environment $1"
    { dda env dev stop --id "$1" && dda env dev remove --id "$1"; } >/dev/null 2>&1
}

dev_env_review() {
    if [[ -f /.started ]]; then
        log "already running in a developer environment"
        return 1
    fi
    if ! command -v docker >/dev/null; then
        log "docker is not installed, which developer environments need"
        return 1
    fi

    # One environment per checkout, reused across runs since creating it pulls a large image
    local env_id state
    env_id="create-pr-review-$(printf '%s' "$repo_root" | cksum | cut -d' ' -f1)"
    state=$(dda env dev status --id "$env_id" 2>/dev/null | sed -n 's/^State: //p')
    if [[ "$state" != "started" ]]; then
        log "starting developer environment ${env_id} (state: ${state:-unknown}), the first start pulls the image"
        dda env dev start --id "$env_id" --repo "$repo_root" >&2 || return 1
    fi

    # Remove an environment missing a prerequisite so the next run recreates it with newer dda and image versions
    local in_env=(dda env dev run --id "$env_id" --)
    if ! "${in_env[@]}" git rev-parse --git-dir >/dev/null 2>&1; then
        log "git does not work in ${env_id}: Git worktrees need dda with DataDog/datadog-agent-dev#320"
        discard_dev_env "$env_id"
        return 1
    fi
    if ! "${in_env[@]}" npm --version >/dev/null 2>&1; then
        log "npm is missing in ${env_id}: it needs an image with DataDog/datadog-agent-buildimages#1367"
        discard_dev_env "$env_id"
        return 1
    fi
    if ! "${in_env[@]}" ddtool auth token rapid-ai-platform --datacenter us1.ddbuild.io >/dev/null 2>&1; then
        log "claude in ${env_id} has no credentials: \`ddtool auth token rapid-ai-platform\` failed"
        return 1
    fi

    log "developer environment ${env_id} is kept for later runs, remove it with \`dda env dev remove --id ${env_id}\`"
    run_review "claude in developer environment ${env_id} (CI uses codex, so findings may differ)" \
        "${in_env[@]}" dda review --provider claude "$@"
}
dev_env_review "$@"

if [[ "$attempted" -eq 1 ]]; then
    exit 1
fi

log "ALERT: no AI reviewer is available, so no local review ran (reasons above). To enable it, either:"
log "  - install codex and run \`codex login\` (same reviewer as CI)"
log "  - install Claude Code and log in"
log "  - fix what prevents running claude in a \`dda env dev\` developer environment, which needs docker"
exit 2
