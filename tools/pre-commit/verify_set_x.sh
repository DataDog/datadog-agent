#!/bin/sh

# Verify no set -x is used in this repository

files="$(git grep -rn --color=always 'set -x' -- ':*.sh' ':*/Dockerfile' ':*.yaml' ":(exclude)$0" ':(exclude).pre-commit-config.yaml')"

if [ -n "$files" ]; then
    echo "$files" >& 2
    echo -e '\nerror: No script should include `set -x`' >& 2
    exit 1
else
    exit 0
fi
