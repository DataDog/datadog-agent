#!/bin/bash
# shellcheck source=/dev/null
source /root/.bashrc
# junit file name can differ in kitchen or macos context
junit_files="junit-*.tgz"
if [[ -n "$1" ]]; then
    junit_files="$1"
fi

# Open file description for secrets
# shellcheck source=/dev/null
source "$CI_PROJECT_DIR"/tools/ci/open_file_descriptor.sh
"$CI_PROJECT_DIR"/tools/ci/aws_ssm_get_wrapper.sh "$GITLAB_READ_API_TOKEN"
"$CI_PROJECT_DIR"/tools/ci/aws_ssm_get_wrapper.sh "$API_KEY_ORG2"
DATADOG_API_KEY=$(pop_ssm)
GITLAB_TOKEN=$(pop_ssm)
export DATADOG_API_KEY
export GITLAB_TOKEN
error=0
for file in $junit_files; do
    if [[ ! -f $file ]]; then
        echo "Issue with junit file: $file"
        continue
    fi
    inv -e junit-upload --tgz-path "$file" || error=1
done
# Never fail on Junit upload failure since it would prevent the other after scripts to run.
if [ $error -eq 1 ]; then
    echo "Error: Junit upload failed"
fi
exit 0
