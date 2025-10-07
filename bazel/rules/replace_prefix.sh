#!/bin/bash

while [ "$#" -gt 0 ]; do
    case "$1" in
        --prefix | -p)
            shift
            PREFIX="$1"
            ;;
        *)
            break
    esac
    shift
done

if [ -z "$PREFIX" ]; then
    echo "Usage: replace_prefix.sh -p <new prefix> <input file>"
    exit 1
fi

for f in "$@"; do
    if [ ! -f "$f" ]; then
        echo "$f: file not found"
        exit 2
    fi
    case $f in
        *.so)
            patchelf --set-rpath "$PREFIX" "$f"
            ;;
        *.dylib)
            install_name_tool -add_rpath "$PREFIX/embedded/lib" "$f"
            ;;
        *.pc)
            sed -i "s|##PREFIX##|$PREFIX|" "$f"
            ;;
        *)
            echo "Ignoring $f"
    esac
done
