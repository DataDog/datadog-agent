#!/bin/bash

# Usage - run commands from repo root:
# To check if new changes to the extension cause changes to any snapshots:
#   BUILD_EXTENSION=true aws-vault exec sandbox-account-admin -- ./run.sh
# To regenerate snapshots:
#   UPDATE_SNAPSHOTS=true aws-vault exec sandbox-account-admin -- ./run.sh

LOGS_WAIT_SECONDS=600

DEFAULT_NODE_LAYER_VERSION=66
DEFAULT_PYTHON_LAYER_VERSION=49

set -e

script_utc_start_time=$(date -u +"%Y%m%dT%H%M%S")

if [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
    echo "No AWS credentials were found in the environment."
    echo "Note that only Datadog employees can run these integration tests."
    exit 1
fi

# Move into the root directory, so this script can be called from any directory
SERVERLESS_INTEGRATION_TESTS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$SERVERLESS_INTEGRATION_TESTS_DIR/../../.."

# TODO: Get this working in CI environment
LAMBDA_EXTENSION_REPOSITORY_PATH="../datadog-lambda-extension"

if [ "$BUILD_EXTENSION" == "true" ]; then
    echo "Building extension that will be deployed with our test functions"

    # This version number is arbitrary and won't be used by AWS
    PLACEHOLDER_EXTENSION_VERSION=123

    ARCHITECTURE=amd64 VERSION=$PLACEHOLDER_EXTENSION_VERSION $LAMBDA_EXTENSION_REPOSITORY_PATH/scripts/build_binary_and_layer_dockerized.sh
else
    echo "Not building extension, ensure it has already been built or re-run with 'BUILD_EXTENSION=true'"
fi

cd "./test/integration/serverless"

# build and zip recorder extension
echo "Building recorder extension"
cd recorder-extension
GOOS=linux GOARCH=amd64 go build -o extensions/recorder-extension main.go
zip -rq ext.zip extensions/* -x ".*" -x "__MACOSX" -x "extensions/.*"
cd ..

# build Go Lambda functions
echo "Building Go Lambda functions"
go_test_dirs=("with-ddlambda" "without-ddlambda" "log-with-ddlambda" "log-without-ddlambda" "timeout" "trace")
cd src
for go_dir in "${go_test_dirs[@]}"; do
    env GOOS=linux go build -ldflags="-s -w" -o bin/"$go_dir" go-tests/"$go_dir"/main.go
done

#build .NET functions
echo "Building .NET Lambda functions"
cd csharp-tests
dotnet restore
set +e #set this so we don't exit if the tools are already installed
dotnet tool install -g Amazon.Lambda.Tools --framework netcoreapp3.1
dotnet lambda package --configuration Release --framework netcoreapp3.1 --output-package bin/Release/netcoreapp3.1/handler.zip
set -e
cd ../../

if [ -z "$NODE_LAYER_VERSION" ]; then
    echo "NODE_LAYER_VERSION not found, using the default"
    export NODE_LAYER_VERSION=$DEFAULT_NODE_LAYER_VERSION
fi

if [ -z "$PYTHON_LAYER_VERSION" ]; then
    echo "PYTHON_LAYER_VERSION not found, using the default"
    export PYTHON_LAYER_VERSION=$DEFAULT_PYTHON_LAYER_VERSION
fi

echo "NODE_LAYER_VERSION set to: $NODE_LAYER_VERSION"
echo "PYTHON_LAYER_VERSION set to: $PYTHON_LAYER_VERSION"

# random 8-character ID to avoid collisions with other runs
stage=$(xxd -l 4 -c 4 -p </dev/random)

function remove_stack() {
    echo "Removing stack for stage : ${stage}"
    serverless remove --stage "${stage}"
}

# always remove the stacks before exiting, no matter what
trap remove_stack EXIT

# deploy the stack
NODE_LAYER_VERSION=${NODE_LAYER_VERSION} \
    PYTHON_LAYER_VERSION=${PYTHON_LAYER_VERSION} \
    serverless deploy --stage "${stage}"

# invoke functions

metric_function_names=("enhanced-metric-node" "enhanced-metric-python" "metric-csharp" "no-enhanced-metric-node" "no-enhanced-metric-python" "with-ddlambda-go" "without-ddlambda-go" "timeout-python" "timeout-node" "timeout-go" "error-python" "error-node")
log_function_names=("log-node" "log-python" "log-csharp" "log-go-with-ddlambda" "log-go-without-ddlambda")
trace_function_names=("simple-trace-node" "simple-trace-python" "simple-trace-go")

all_functions=("${metric_function_names[@]}" "${log_function_names[@]}" "${trace_function_names[@]}")

set +e # Don't exit this script if an invocation fails or there's a diff
for function_name in "${all_functions[@]}"; do
    serverless invoke --stage "${stage}" -f "${function_name}"
done
#wait 30 seconds to make sure metrics aren't merged into a single metric
sleep 30
for function_name in "${all_functions[@]}"; do
    # two invocations are needed since enhanced metrics are computed with the REPORT log line (which is created at the end of the first invocation)
    return_value=$(serverless invoke --stage "${stage}" -f "${function_name}")
    # Compare new return value to snapshot
    diff_output=$(echo "$return_value" | diff - "./snapshots/expectedInvocationResult")
    if [ "$?" -eq 1 ] && { [ "${function_name:0:7}" != timeout ] && [ "${function_name:0:5}" != error ]; }; then
        echo "Failed: Return value for $function_name does not match snapshot:"
        echo "$diff_output"
        mismatch_found=true
    else
        echo "Ok: Return value for $function_name matches snapshot"
    fi
done

now=$(date +"%r")
echo "Sleeping $LOGS_WAIT_SECONDS seconds to wait for logs to appear in CloudWatch..."
echo "This should be done in 10 minutes from $now"

sleep $LOGS_WAIT_SECONDS

for function_name in "${all_functions[@]}"; do
    echo "Fetching logs for ${function_name} on ${stage}"
    retry_counter=0
    while [ $retry_counter -lt 10 ]; do
        raw_logs=$(NODE_LAYER_VERSION=${NODE_LAYER_VERSION} PYTHON_LAYER_VERSION=${PYTHON_LAYER_VERSION} serverless logs --stage "${stage}" -f "$function_name" --startTime "$script_utc_start_time")
        fetch_logs_exit_code=$?
        if [ $fetch_logs_exit_code -eq 1 ]; then
            echo "Retrying fetch logs for $function_name..."
            retry_counter=$(($retry_counter + 1))
            sleep 10
            continue
        fi
        break
    done

    # Replace invocation-specific data like timestamps and IDs with XXX to normalize across executions
    if [[ " ${metric_function_names[*]} " =~ " ${function_name} " ]]; then
        # Normalize metrics
        logs=$(
            echo "$raw_logs" |
                perl -p -e "s/raise Exception/\n/g" |
                grep -v "\[log\]" |
                grep "\[sketch\].*" |
                perl -p -e "s/(ts\":)[0-9]{10}/\1XXX/g" |
                perl -p -e "s/(min\":)[0-9\.e\-]{1,30}/\1XXX/g" |
                perl -p -e "s/(max\":)[0-9\.e\-]{1,30}/\1XXX/g" |
                perl -p -e "s/(cnt\":)[0-9\.e\-]{1,30}/\1XXX/g" |
                perl -p -e "s/(avg\":)[0-9\.e\-]{1,30}/\1XXX/g" |
                perl -p -e "s/(sum\":)[0-9\.e\-]{1,30}/\1XXX/g" |
                perl -p -e "s/(k\":\[)[0-9\.e\-]{1,30}/\1XXX/g" |
                perl -p -e "s/(datadog-nodev)[0-9]+\.[0-9]+\.[0-9]+/\1X\.X\.X/g" |
                perl -p -e "s/(datadog_lambda:v)[0-9]+\.[0-9]+\.[0-9]+/\1X\.X\.X/g" |
                perl -p -e "s/dd_lambda_layer:datadog-go[0-9.]{1,}/dd_lambda_layer:datadog-gox.x.x/g" |
                perl -p -e "s/(dd_lambda_layer:datadog-python)[0-9_]+\.[0-9]+\.[0-9]+/\1X\.X\.X/g" |
                perl -p -e "s/(serverless.lambda-extension.integration-test.count)[0-9\.]+/\1/g" |
                perl -p -e "s/$stage/XXXXXX/g" |
                perl -p -e "s/[ ]$//g" |
                sort
        )
    elif [[ " ${log_function_names[*]} " =~ " ${function_name} " ]]; then
        # Normalize logs
        logs=$(
            echo "$raw_logs" |
                grep "\[log\]" |
                perl -p -e "s/(timestamp\":)[0-9]{13}/\1TIMESTAMP/g" |
                perl -p -e "s/(\"REPORT |START |END ).*/\1XXX\"}}/g" |
                perl -p -e "s/(\"HTTP ).*/\1\"}}/g" |
                perl -p -e "s/(,\"request_id\":\")[a-zA-Z0-9\-,]+\"//g" |
                perl -p -e "s/$stage/STAGE/g" |
                perl -p -e "s/(\"message\":\").*(XXX LOG)/\1\2\3/g" |
                perl -p -e "s/[ ]$//g" |
                grep XXX
        )
    else
        # Normalize traces
        logs=$(
            echo "$raw_logs" |
                grep "\[trace\]" |
                perl -p -e "s/(ts\":)[0-9]{10}/\1XXX/g" |
                perl -p -e "s/((startTime|endTime|traceID|trace_id|span_id|parent_id|start|system.pid)\":)[0-9]+/\1XXX/g" |
                perl -p -e "s/(duration\":)[0-9]+/\1XXX/g" |
                perl -p -e "s/((datadog_lambda|dd_trace)\":\")[0-9]+\.[0-9]+\.[0-9]+/\1X\.X\.X/g" |
                perl -p -e "s/(,\"request_id\":\")[a-zA-Z0-9\-,]+\"/\1XXX\"/g" |
                perl -p -e "s/(,\"runtime-id\":\")[a-zA-Z0-9\-,]+\"/\1XXX\"/g" |
                perl -p -e "s/(,\"system.pid\":\")[a-zA-Z0-9\-,]+\"/\1XXX\"/g" |
                perl -p -e "s/$stage/XXXXXX/g" |
                perl -p -e "s/[ ]$//g" |
                sort
        )
    fi

    function_snapshot_path="./snapshots/${function_name}"

    if [ ! -f "$function_snapshot_path" ]; then
        # If no snapshot file exists yet, we create one
        echo "Writing logs to $function_snapshot_path because no snapshot exists yet"
        echo "$logs" > "$function_snapshot_path"
    elif [ "$UPDATE_SNAPSHOTS" == "true" ]; then
        # If $UPDATE_SNAPSHOTS is set to true write the new logs over the current snapshot
        echo "Overwriting log snapshot for $function_snapshot_path"
        echo "$logs" > "$function_snapshot_path"
    else
        # Compare new logs to snapshots
        diff_output=$(echo "$logs" | diff - "$function_snapshot_path")
        if [ $? -eq 1 ]; then
            echo "Failed: Mismatch found between new $function_name logs (first) and snapshot (second):"
            echo "$diff_output"
            mismatch_found=true
        else
            echo "Ok: New logs for $function_name match snapshot"
        fi
    fi

done

echo
if [ "$UPDATE_SNAPSHOTS" == "true" ]; then
    echo "DONE: Snapshots were updated for all functions."
    exit 0
elif [ "$mismatch_found" == true ]; then
    echo "FAILURE: A mismatch between new data and a snapshot was found and printed above."
    exit 1
fi

echo "SUCCESS: No difference found between snapshots and new return values or logs"
echo
