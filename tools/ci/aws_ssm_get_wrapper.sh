#!/bin/bash

retry_count=0
max_retries=10
parameter_name="$1"

source /root/.bashrc > /dev/null 2>&1

set +x

while [[ $retry_count -lt $max_retries ]]; do
    result=$(aws ssm get-parameter --region us-east-1 --name $parameter_name --with-decryption --query "Parameter.Value" --output text)
    if [ -n "$result" ]; then
        echo "$result"
        exit 0
    fi
    retry_count=$((retry_count+1))
    sleep $((2**retry_count))
done

echo "Failed to retrieve parameter after $max_retries retries"
exit 1
