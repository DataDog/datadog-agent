#!/bin/bash
set -e

# Detect if we're in a worktree and get container ID
get_container_id() {
    if [ -d .git ]; then
        echo "default"
    elif [ -f .git ]; then
        basename "$(pwd)"
    else
        echo "Error: Not in a git repository" >&2
        exit 1
    fi
}

# Ensure dev container is running
ensure_container_running() {
    local container_id="$1"
    local status_output

    echo "🔍 Checking dev container status (ID: $container_id)..."

    status_output=$(dda env dev status --id "$container_id" 2>/dev/null || echo "")

    if echo "$status_output" | grep -qi "started"; then
        echo "✓ Dev container is already running"
        return 0
    fi

    echo "🚀 Starting dev container..."

    # Only use --no-pull if container doesn't exist (nonexistent state)
    if echo "$status_output" | grep -q "nonexistent"; then
        if ! dda env dev start --id "$container_id" --no-pull; then
            echo "❌ Failed to start dev container" >&2
            exit 1
        fi
    else
        # Container exists but is stopped - don't use --no-pull
        if ! dda env dev start --id "$container_id"; then
            echo "❌ Failed to start dev container" >&2
            exit 1
        fi
    fi

    echo "✓ Dev container started successfully"
}

# Main execution
main() {
    local container_id
    container_id=$(get_container_id)

    ensure_container_running "$container_id"

    echo "🔨 Building Datadog Agent in container (ID: $container_id)..."
    dda env dev run --id "$container_id" -- dda inv agent.build "$@"

    echo "✅ Build completed successfully"
}

main "$@"
