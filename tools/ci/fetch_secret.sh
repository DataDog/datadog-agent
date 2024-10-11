#!/bin/bash

retry_count=0
max_retries=10
parameter_name="$1"
parameter_field="$2"

set +x

while [[ $retry_count -lt $max_retries ]]; do
    if [ -n "$parameter_field" ]; then
        result=$(vault kv get -field="${parameter_field}" kv/k8s/gitlab-runner/datadog-agent/"${parameter_name}" 2> errorFile)
    else
        result=$(aws ssm get-parameter --region us-east-1 --name "$parameter_name" --with-decryption --query "Parameter.Value" --output text 2> errorFile)
    fi
    error=$(<errorFile)
    if [ -n "$result" ]; then
        echo "$result"
        exit 0
    fi
    if [[ "$error" =~ "Unable to locate credentials" ]]; then
        # See 5th row in https://docs.google.com/spreadsheets/d/1JvdN0N-RdNEeOJKmW_ByjBsr726E3ZocCKU8QoYchAc
        >&2 echo "Permanent error: unable to locate credentials, not retrying"
        exit 42
    fi
    retry_count=$((retry_count+1))
    sleep $((2**retry_count))
done

echo "Failed to retrieve parameter after $max_retries retries"
exit 1
