#!/usr/bin/env bash
#
# Usage: wine_run <path/to/windows-executable> [args...]
#
# Each invocation gets a throwaway prefix, so no state leaks between runs or
# from the user's ~/.wine.

# --- begin runfiles.bash initialization v3 ---
set -uo pipefail; set +e; f=bazel_tools/tools/bash/runfiles/runfiles.bash
# shellcheck disable=SC1090
source "${RUNFILES_DIR:-/dev/null}/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "${RUNFILES_MANIFEST_FILE:-/dev/null}" | cut -f2- -d' ')" 2>/dev/null || \
  source "$0.runfiles/$f" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  source "$(grep -sm1 "^$f " "$0.exe.runfiles_manifest" | cut -f2- -d' ')" 2>/dev/null || \
  { echo>&2 "ERROR: cannot find $f"; exit 1; }; f=; set -e
# --- end runfiles.bash initialization v3 ---

# rlocation is relative when the tool is invoked by a relative path (as in a
# Bazel action), and Wine changes directory before spawning wineserver/wineboot.
resolve() { realpath "$(rlocation "$1")"; }

wine="$(resolve wine_linux_x86_64/bin/wine)"
wineserver="$(resolve wine_linux_x86_64/bin/wineserver)"

emulator=()
if [[ "$(uname -m)" == aarch64 ]]; then
    # box64 also translates the wineserver/wineboot processes Wine spawns.
    emulator=("$(resolve box64_linux_aarch64/usr/bin/box64)")
    # Use the pinned rcfile instead of the host's /etc/box64.box64rc.
    BOX64_RCFILE="$(resolve box64_linux_aarch64/etc/box64.box64rc)"
    # Only the pinned x86_64 libraries, never the host's /usr/lib/x86_64-linux-gnu.
    BOX64_LD_LIBRARY_PATH="$(dirname "$(resolve libgcc_s_linux_x86_64/usr/lib/x86_64-linux-gnu/libgcc_s.so.1)")"
    export BOX64_RCFILE BOX64_LD_LIBRARY_PATH BOX64_LOG=0 BOX64_NOBANNER=1
fi

scratch="$(mktemp -d "${TMPDIR:-/tmp}/wine_run.XXXXXX")"
cleanup() {
    "${emulator[@]}" "$wineserver" -k -w 2>/dev/null || true
    rm -rf "$scratch"
}
trap cleanup EXIT

export HOME="$scratch"
export XDG_CACHE_HOME="$scratch/cache"
export WINEPREFIX="$scratch/prefix"
export WINEARCH=win64
export WINESERVER="$wineserver"
export WINEDEBUG="${WINEDEBUG:--all}"
# Skip the Mono/Gecko installers and menu integration during prefix creation.
export WINEDLLOVERRIDES="mscoree,mshtml,winemenubuilder.exe="
export LANG=C.UTF-8 LC_ALL=C.UTF-8 TZ=UTC
unset DISPLAY WAYLAND_DISPLAY

# wineserver SIGKILLs a process whose Unix side outlives its Windows exit by
# ~1s, which box64 teardown of large programs routinely does. start.exe waits
# for the program and exits with its Windows exit code; /d . keeps the cwd.
"${emulator[@]}" "$wine" start /wait /b /d . /unix "$@"
