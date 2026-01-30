#!/bin/bash
set -e

# Get path relative to main repo root
get_relative_path() {
    local common_git_dir
    local main_repo_root
    local current_dir
    local relative_path

    # Get the common git directory (main repo's .git)
    common_git_dir=$(git rev-parse --git-common-dir)
    # Main repo root is parent of .git
    main_repo_root=$(dirname "$common_git_dir")
    current_dir=$(git rev-parse --show-toplevel)

    # Get relative path from main repo root to current worktree
    relative_path="${current_dir#"$main_repo_root"}"

    # Remove leading slash if present
    relative_path="${relative_path#/}"

    # If empty, we're at the main repo root
    if [ -z "$relative_path" ]; then
        echo "."
    else
        echo "$relative_path"
    fi
}

# Main execution
main() {
    local relative_path
    relative_path=$(get_relative_path)

    echo "Building Datadog Agent..."
    echo "Working directory: $relative_path"

    if [ "$relative_path" = "." ]; then
        dda env dev run -- dda inv agent.build "$@"
    else
        dda env dev run -- bash -c "cd '$relative_path' && dda inv agent.build $*"
    fi

    echo "Build completed successfully"
}

main "$@"
