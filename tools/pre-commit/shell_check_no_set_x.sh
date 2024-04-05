#!/bin/sh

# Verify no set -x is used in this repository

files="$(git grep -rnE --color=always 'set( +-[^ ])* +-[^ ]*(x|( +xtrace))' -- ':*.sh' ':*/Dockerfile' ':*.yaml' ':*.yml' ":(exclude)$0" ':(exclude).pre-commit-config.yaml')"

if [ -n "$files" ]; then
    echo "$files" >& 2
    echo >& 2
    echo 'error: No script should include "set -x"' >& 2
    exit 1
else
    exit 0
fi
