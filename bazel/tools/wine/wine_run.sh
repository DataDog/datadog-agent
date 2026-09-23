#!/usr/bin/env bash
#
# Runs a Windows executable under Wine, so that Windows-only code generators can
# be executed from a Linux or macOS host.
#
# Usage: wine_run <windows-executable> [args...]
#
# Only the `wine` package is needed; the binary is invoked explicitly, so the
# binfmt_misc registration provided by `wine-binfmt` / `binfmt-support` is not
# required.

set -euo pipefail

if ! command -v wine >/dev/null 2>&1; then
    echo "wine_run: 'wine' was not found on PATH." >&2
    echo "wine_run: install it (e.g. 'sudo apt install wine64') to generate Windows outputs from this host." >&2
    exit 1
fi

# Bazel scrubs the action environment, so HOME is usually unset or not writable.
# Wine insists on a writable prefix, and fontconfig on a writable cache, so point
# both at the action's temporary directory.
if [[ ! -w "${HOME:-/nonexistent}" ]]; then
    scratch="${TMPDIR:-/tmp}/wine_run"
    : "${WINEPREFIX:="${scratch}/prefix"}"
    : "${XDG_CACHE_HOME:="${scratch}/cache"}"
    export WINEPREFIX XDG_CACHE_HOME
    mkdir -p "${WINEPREFIX}" "${XDG_CACHE_HOME}"
fi

# Wine is chatty on stderr and its fixme/err noise makes generator output hard to
# read; keep it quiet unless the caller asked for detail.
export WINEDEBUG="${WINEDEBUG:--all}"

exec wine "$@"
