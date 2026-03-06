#!/bin/bash
# Shared library for running dda inv commands in dev container

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

# Run dda inv command in container
# Usage: run_inv_in_container <command> <args...>
run_inv_in_container() {
    local inv_command="$1"
    shift
    local relative_path
    relative_path=$(get_relative_path)

    echo "Working directory: $relative_path"

    if [ "$relative_path" = "." ]; then
        dda env dev run -- dda inv "$inv_command" "$@"
    else
        dda env dev run -- bash -c "cd '$relative_path' && dda inv $inv_command $*"
    fi
}
