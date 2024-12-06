#!/bin/bash
# shellcheck source=/dev/null
# junit file name can differ in kitchen or macos context
junit_files="junit-*.tgz"
if [[ -n "$1" ]]; then
    junit_files="$1"
fi

DATADOG_API_KEY="$("$CI_PROJECT_DIR"/tools/ci/fetch_secret.sh "$AGENT_API_KEY_ORG2" token)"
export DATADOG_API_KEY
error=0
for file in $junit_files; do
    if [[ ! -f $file ]]; then
        echo "Issue with junit file: $file"
        continue
    fi
    inv -e junit-upload --tgz-path "$file" || error=1
done
unset DATADOG_API_KEY
# Never fail on Junit upload failure since it would prevent the other after scripts to run.
if [ $error -eq 1 ]; then
    echo "Error: Junit upload failed"
fi
exit 0
