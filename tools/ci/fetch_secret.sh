#!/bin/bash

# Usage: fetch_secret.sh <param-name> <param-field> [<format>]

retry_count=0
max_retries=10
parameter_name="$1"
parameter_field="$2"
format="${3:-table}"

set +x

while [[ $retry_count -lt $max_retries ]]; do
    exit_code=0
    if [ -n "$parameter_field" ]; then
        # Using Vault; format parameter is respected
        vault_name="kv/k8s/${POD_NAMESPACE}/datadog-agent"
        if [[ "$(uname -s)" == "Darwin" ]]; then
            vault_name="kv/aws/arn:aws:iam::486234852809:role/ci-datadog-agent"
            result="$(ci-identities-gitlab-job-client secrets read ${parameter_name} ${parameter_field} 2> errorFile)"
            exit_code=$?
        else
            result="$(vault kv get -format="${format}" -field="${parameter_field}" "${vault_name}"/"${parameter_name}" 2> errorFile)"
            exit_code=$?
        fi
    else
        # Using SSM; the [<format>] parameter is ignored
        if [[ "$(uname -s)" == "Darwin" ]]; then
            if [ -z "$AWS_SHARED_CREDENTIALS_FILE" ]; then
                echo "Error: AWS_SHARED_CREDENTIALS_FILE is not set when using CI Identities for AWS Credentials" >&2
                exit 1
            fi
            ci-identities-gitlab-job-client assume-role
        fi
        result="$(aws ssm get-parameter --region us-east-1 --name "$parameter_name" --with-decryption --query "Parameter.Value" --output text 2> errorFile)"
        exit_code=$?
    fi
    error="$(<errorFile)"
    if [ $exit_code -eq 0 ] && [ -n "$result" ]; then
        echo "$result"
        exit 0
    else
        echo "$error" >&2
    fi
    if [[ "$error" =~ "Unable to locate credentials" ]]; then
        # This error needs a restart of the job
        echo "Permanent error: unable to locate credentials, not retrying" >&2
        exit 42
    fi
    retry_count="$((retry_count+1))"
    sleep "$((2**retry_count))"
done

echo "Failed to retrieve parameter after $max_retries retries" >&2
exit 1
