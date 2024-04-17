#!/bin/bash
# shellcheck source=/dev/null
source /root/.bashrc
# junit file name can differ in kitchen or macos context
junit_files="junit-*.tar.gz"
if [[ -n "$1" ]]; then
    junit_files="$1"
fi

DATADOG_API_KEY="$("$CI_PROJECT_DIR"/tools/ci/aws_ssm_get_wrapper.sh "$API_KEY_ORG2_SSM_NAME")"
export DATADOG_API_KEY
error=0
for file in $junit_files; do
    if [[ ! -f $file ]]; then
        echo "Issue with junit file: $file"
        continue
    fi
    inv -e junit-upload --tgz-path "$file" || error=1
done
exit $error
