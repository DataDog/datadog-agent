#!/usr/bin/env bash
# Fail if the install prefix the omnibus build populated holds a symlink that is dangling, points
# outside the prefix, or is absolute. Same check as the one CI runs after the omnibus build
# (.gitlab/build/package_build/build_agent_dmg.sh), so a local build catches it before CI does.
#
# Usage: check_install_symlinks.sh <INSTALL_DIR>
set -eo pipefail

install_dir="${1:?usage: $0 <INSTALL_DIR>}"
declare -i bad=0
real_install=$(readlink -f "$install_dir")
while read -r link; do
    target=$(readlink "$link")
    if [ ! -e "$link" ]; then
        bad+=1
        echo >&2 "Dangling symlink: $link -> $target (must resolve to an existing target)"
        continue
    fi
    real_target=$(readlink -f "$link")
    if [[ ! $real_target = $real_install/* ]]; then
        bad+=1
        echo >&2 "Outbound symlink: $link -> $target (must resolve inside install prefix)"
        continue
    fi
    if [[ $target = /* ]]; then
        bad+=1
        echo >&2 "Absolute symlink: $link -> $target (must be relative to symlink's directory)"
    fi
done < <(find "$install_dir" -type l)
exit $bad
