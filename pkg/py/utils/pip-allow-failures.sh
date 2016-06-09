#!/bin/sh

PIP_COMMAND=${PIP_COMMAND:-pip}
PIP_OPTIONS=${PIP_OPTIONS:-}

while read dependency; do
    dependency_stripped="$(echo "${dependency}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    case "$dependency_stripped" in
    # Skip comments
    \#*)
        continue
        ;;
    # Skip blank lines
    "")
        continue
        ;;
    *)
        if $PIP_COMMAND install $PIP_OPTIONS "$dependency_stripped" 2>&1; then
            echo "$dependency_stripped is installed"
        else
            echo "Could not install $dependency_stripped, skipping"
        fi
        ;;
    esac
done < "$1"
