#!/usr/bin/env bash

set -exo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd $ROOT

bash $ROOT/tools/licenses/license.sh

DIFF=$(git --no-pager diff $ROOT/LICENSE-3rdparty.csv)
if [[ "${DIFF}x" != "x" ]]
then
    echo "License outdated:"
    git --no-pager diff $ROOT/LICENSE-3rdparty.csv
    exit 2
fi

DIFF=$(git ls-files docs/ --exclude-standard --others)
if [[ "${DIFF}x" != "x" ]]
then
    echo "License removed:"
    echo ${DIFF}
    exit 2
fi
echo "Licenses ok"
exit 0
