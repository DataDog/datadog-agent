#!/usr/bin/env bash

set -exo pipefail

export LC_ALL=C

cd $(dirname $0)/../..
ROOT=$(pwd)
LICENSE_FILE=$ROOT/LICENSE-3rdparty.csv

echo Component,Origin,License > $LICENSE_FILE
echo 'core,"github.com/frapposelli/wwhrd",MIT' >> $LICENSE_FILE
$ROOT/bin/wwhrd list --no-color |& grep "Found License" | awk '{print $6,$5}' | sed -E "s/\x1B\[([0-9]{1,2}(;[0-9]{1,2})?)?[mGK]//g" | sed s/" license="/,/ | sed s/package=/core,/ | sort >> $LICENSE_FILE