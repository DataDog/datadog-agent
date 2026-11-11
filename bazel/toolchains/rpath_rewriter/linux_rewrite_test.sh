#!/usr/bin/env bash
set -euo pipefail

readelf="$1"
shift

find -L "$@" -name '*.so' -print0 > "${TEST_TMPDIR}/libs"
mapfile -d '' -t libs < "${TEST_TMPDIR}/libs"
[ "${#libs[@]}" -gt 0 ] || { echo "FAIL: no libraries found"; exit 1; }

failed=0
for lib in "${libs[@]}"; do
    if "$readelf" --dynamic "$lib" | grep -E '\((RPATH|RUNPATH)\)' > /dev/null; then
        has_rpath=1
    else
        has_rpath=0
    fi

    case "$(basename "$lib")" in
        libplain.so) expected=0 ;;
        *) expected=1 ;;
    esac

    if [ "$has_rpath" -eq "$expected" ]; then
        echo "OK: ${lib} (rpath: ${has_rpath})"
    else
        echo "FAIL: ${lib} (rpath: ${has_rpath}, expected: ${expected})"
        failed=1
    fi
done

exit "$failed"
