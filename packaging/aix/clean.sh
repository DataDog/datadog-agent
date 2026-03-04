#!/bin/sh
# clean.sh — reset build state for a clean rebuild while preserving expensive caches
#
# By default, removes:
#   - All stage sentinel files ($BUILD_DIR/.done/)
#   - The staging tree ($BUILD_DIR/staging/)
#   - Build logs ($BUILD_DIR/logs/)
#   - Temporary packaging files ($BUILD_DIR/.pkg_filelist.tmp, etc.)
#
# By default, PRESERVES:
#   - Wheel cache ($BUILD_DIR/wheel-cache/) — pydantic-core (52-min Rust build),
#     cryptography
#   - integrations-core checkout ($BUILD_DIR/integrations-core/) — avoids re-clone
#
# Usage:
#   ./clean.sh              — clean build state, preserve wheel cache
#   ./clean.sh --full       — also remove wheel cache (forces Rust rebuild next run)
#
# After running, re-run build.sh to perform a fresh build.

set -eu

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/lib/env.sh"

FULL_CLEAN=0
for arg in "$@"; do
    case "$arg" in
        --full)
            FULL_CLEAN=1
            ;;
        -h|--help)
            printf 'Usage: %s [--full]\n' "$0"
            printf '  (no args)  Clean build state; preserve wheel cache and integrations-core\n'
            printf '  --full     Also remove wheel cache (forces 52-min pydantic-core Rust rebuild)\n'
            exit 0
            ;;
        *)
            printf 'ERROR: unknown option: %s\n' "$arg" >&2
            exit 1
            ;;
    esac
done

printf '=== Datadog Agent AIX build clean ===\n'
printf '    BUILD_DIR   = %s\n' "$BUILD_DIR"
printf '    STAGING     = %s\n' "$STAGING"
printf '    WHEEL_CACHE = %s\n' "$WHEEL_CACHE"
printf '\n'

# ── Remove stage sentinels ─────────────────────────────────────────────────────
if [ -d "$BUILD_DIR/.done" ]; then
    _count=$(find "$BUILD_DIR/.done" -type f | wc -l | tr -d ' ')
    printf 'Removing %s stage sentinel(s) from %s/.done/\n' "$_count" "$BUILD_DIR"
    rm -rf "$BUILD_DIR/.done"
else
    printf 'No sentinels directory found — already clean.\n'
fi

# ── Remove staging tree ────────────────────────────────────────────────────────
# This removes all compiled libraries, Python installation, and the agent binary.
# The next build will recompile everything from source (except wheel cache below).
if [ -d "$STAGING" ]; then
    printf 'Removing staging tree: %s\n' "$STAGING"
    rm -rf "$STAGING"
else
    printf 'Staging tree not found — already clean.\n'
fi

# ── Remove build logs ──────────────────────────────────────────────────────────
if [ -d "$BUILD_DIR/logs" ]; then
    printf 'Removing build logs: %s/logs/\n' "$BUILD_DIR"
    rm -rf "$BUILD_DIR/logs"
fi

# ── Remove temporary packaging files ──────────────────────────────────────────
rm -f "$BUILD_DIR/.pkg_filelist.tmp"

# ── Wheel cache ────────────────────────────────────────────────────────────────
if [ "$FULL_CLEAN" -eq 1 ]; then
    if [ -d "$WHEEL_CACHE" ]; then
        printf 'FULL CLEAN: removing wheel cache: %s\n' "$WHEEL_CACHE"
        rm -rf "$WHEEL_CACHE"
    fi
    printf 'FULL CLEAN: next build will recompile pydantic-core from Rust source (~52 min)\n'
else
    if [ -d "$WHEEL_CACHE" ]; then
        printf 'Wheel cache preserved: %s\n' "$WHEEL_CACHE"
    fi
    if [ -d "$BUILD_DIR/integrations-core" ]; then
        printf 'integrations-core checkout preserved: %s/integrations-core\n' "$BUILD_DIR"
    fi
fi

printf '\n=== Clean complete. Run build.sh to rebuild from scratch. ===\n'
