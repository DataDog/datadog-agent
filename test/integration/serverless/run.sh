#!/bin/bash

# Runs Datadog Lambda Extension integration tests.
# NOTE: Use aws-vault clear before running tests to ensure credentials do not expire during a test run

# Run tests:
#   aws-vault clear && aws-vault exec sandbox-account-admin -- ./run.sh
# Regenerate snapshots:
#   aws-vault clear && UPDATE_SNAPSHOTS=true aws-vault exec sandbox-account-admin -- ./run.sh

# Optional environment variables:

# UPDATE_SNAPSHOTS [true|false] - Use this when you want to update snapshots instead of running tests. The default is false.
# BUILD_EXTENSION [true|false] - Whether to build the extension or re-use the previous build. The default is true.
# NODE_LAYER_VERSION [number] - A specific layer version of datadog-lambda-js to use.
# PYTHON_LAYER_VERSION [number] - A specific layer version of datadog-lambda-py to use.
# JAVA_TRACE_LAYER_VERSION [number] - A specific layer version of dd-trace-java to use.

DEFAULT_NODE_LAYER_VERSION=67
DEFAULT_PYTHON_LAYER_VERSION=50
DEFAULT_JAVA_TRACE_LAYER_VERSION=4

# Text formatting constants
RED="\e[1;41m"
GREEN="\e[1;42m"
YELLOW="\e[1;43m"
MAGENTA="\e[1;45m"
END_COLOR="\e[0m"

set -e

script_utc_start_time=$(date -u +"%Y%m%dT%H%M%S")

if [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
    echo "No AWS credentials were found in the environment."
    echo "Note that only Datadog employees can run these integration tests."
    echo "Exiting without running tests..."

    # If credentials are not available, the run is considered a success
    # so as not to break CI for external users that don't have access to GitHub secrets
    exit 0
fi

# Move into the root directory, so this script can be called from any directory
SERVERLESS_INTEGRATION_TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

cd "$SERVERLESS_INTEGRATION_TESTS_DIR/../../.."
LAMBDA_EXTENSION_REPOSITORY_PATH="../datadog-lambda-extension"
if [ "$BUILD_EXTENSION" != "false" ]; then
    echo "Building extension"

    # This version number is arbitrary and won't be used by AWS
    PLACEHOLDER_EXTENSION_VERSION=123

    ARCHITECTURE=amd64 VERSION=$PLACEHOLDER_EXTENSION_VERSION $LAMBDA_EXTENSION_REPOSITORY_PATH/scripts/build_binary_and_layer_dockerized.sh
else
    echo "Skipping extension build, reusing previously built extension"
fi

cd $SERVERLESS_INTEGRATION_TESTS_DIR

./build_recorder.sh
./build_go_functions.sh
./build_java_functions.sh
./build_csharp_functions.sh

if [ -z "$NODE_LAYER_VERSION" ]; then
    export NODE_LAYER_VERSION=$DEFAULT_NODE_LAYER_VERSION
fi

if [ -z "$PYTHON_LAYER_VERSION" ]; then
    export PYTHON_LAYER_VERSION=$DEFAULT_PYTHON_LAYER_VERSION
fi

if [ -z "$JAVA_TRACE_LAYER_VERSION" ]; then
    export JAVA_TRACE_LAYER_VERSION=$DEFAULT_JAVA_TRACE_LAYER_VERSION
fi

echo "Using Node layer version: $NODE_LAYER_VERSION"
echo "Using Python layer version: $PYTHON_LAYER_VERSION"
echo "Using Java tracer layer version: $JAVA_TRACE_LAYER_VERSION"

# random 8-character ID to avoid collisions with other runs
stage=$(xxd -l 4 -c 4 -p </dev/random)

function remove_stack() {
    echo "Removing stack"
    serverless remove --stage "${stage}"
}

# always remove the stack before exiting, no matter what
trap remove_stack EXIT

# deploy the stack
NODE_LAYER_VERSION=${NODE_LAYER_VERSION} \
    PYTHON_LAYER_VERSION=${PYTHON_LAYER_VERSION} \
    serverless deploy --stage "${stage}"

metric_functions=(
    "metric-node"
    "metric-python"
    "metric-java"
    "metric-go"
    "metric-csharp"
    "timeout-node"
    "timeout-python"
    "timeout-java"
    "timeout-go"
    "error-node"
    "error-python"
    "error-java"
)
log_functions=(
    "log-node"
    "log-python"
    "log-java"
    "log-go"
    "log-csharp"
)
trace_functions=(
    "trace-node"
    "trace-python"
    "trace-java"
    "trace-go"
)

all_functions=("${metric_functions[@]}" "${log_functions[@]}" "${trace_functions[@]}")

# Add a function to this list to skip checking its results
# This should only be used temporarily while we investigate and fix the test
functions_to_skip=(
    # Not currently skipping any functions
)

echo "Invoking functions for the first time..."
set +e # Don't exit this script if an invocation fails or there's a diff
for function_name in "${all_functions[@]}"; do
    serverless invoke --stage "${stage}" -f "${function_name}" >/dev/null &
done
wait

# wait to make sure metrics aren't merged into a single metric
SECONDS_BETWEEN_INVOCATIONS=30
echo "Waiting $SECONDS_BETWEEN_INVOCATIONS seconds before invoking functions for the second time..."
sleep $SECONDS_BETWEEN_INVOCATIONS

# two invocations are needed since enhanced metrics are computed with the REPORT log line (which is created at the end of the first invocation)
echo "Invoking functions for the second time..."
for function_name in "${all_functions[@]}"; do
    serverless invoke --stage "${stage}" -f "${function_name}" >/dev/null &
done
wait

LOGS_WAIT_MINUTES=8
END_OF_WAIT_TIME=$(date --date="+"$LOGS_WAIT_MINUTES" minutes" +"%r")
echo "Waiting $LOGS_WAIT_MINUTES minutes for logs to flush..."
echo "This will be done at $END_OF_WAIT_TIME"
sleep "$LOGS_WAIT_MINUTES"m

failed_functions=()

for function_name in "${all_functions[@]}"; do
    echo "Fetching logs for ${function_name}..."
    retry_counter=1
    while [ $retry_counter -lt 11 ]; do
        raw_logs=$(NODE_LAYER_VERSION=${NODE_LAYER_VERSION} PYTHON_LAYER_VERSION=${PYTHON_LAYER_VERSION} serverless logs --stage "${stage}" -f "$function_name" --startTime "$script_utc_start_time")
        fetch_logs_exit_code=$?
        if [ $fetch_logs_exit_code -eq 1 ]; then
            printf "\e[A\e[K" # erase previous log line
            echo "Retrying fetch logs for $function_name... (retry #$retry_counter)"
            retry_counter=$(($retry_counter + 1))
            sleep 10
            continue
        fi
        break
    done
    printf "\e[A\e[K" # erase previous log line

    # Replace invocation-specific data like timestamps and IDs with XXX to normalize across executions
    if [[ " ${metric_functions[*]} " =~ " ${function_name} " ]]; then
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
    elif [[ " ${log_functions[*]} " =~ " ${function_name} " ]]; then
        # Normalize logs
        logs=$(
            echo "$raw_logs" |
                grep -v "\[sketch\]" |
                grep "\[log\]" |
                perl -p -e "s/(timestamp\":)[0-9]{13}/\1TIMESTAMP/g" |
                perl -p -e "s/\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}/\1TIMESTAMP/g" |
                perl -p -e "s/(\"REPORT |START |END ).*/\1XXX\"}}/g" |
                perl -p -e "s/(,\"request_id\":\")[a-zA-Z0-9\-,]+\"//g" |
                perl -p -e "s/$stage/STAGE/g" |
                perl -p -e "s/(\"message\":\").*(XXX LOG)/\1\2\3/g" |
                perl -p -e "s/[ ]$//g"
        )
    else
        # Normalize traces
        logs=$(
            echo "$raw_logs" |
                grep -v "\[log\]" |
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
        printf "${MAGENTA} CREATE ${END_COLOR} $function_name\n"
        echo "$logs" > "$function_snapshot_path"
    elif [ "$UPDATE_SNAPSHOTS" == "true" ]; then
        printf "${MAGENTA} UPDATE ${END_COLOR} $function_name\n"
        echo "$logs" >"$function_snapshot_path"
    else
        if [[ " ${functions_to_skip[*]} " =~ " ${function_name} " ]]; then
            printf "${YELLOW} SKIP ${END_COLOR} $function_name\n"
            continue
        fi
        diff_output=$(echo "$logs" | diff - "$function_snapshot_path")
        if [ $? -eq 1 ]; then
            failed_functions+=("$function_name")

            echo
            printf "${RED} FAIL ${END_COLOR} $function_name\n"
            echo
            echo "Expected logs from snapshot:"
            echo
            cat $function_snapshot_path
            echo
            echo "Actual logs:"
            echo
            echo "$logs"
            echo
            echo "Diff:"
            echo
            echo "$diff_output"
            echo
        else
            printf "${GREEN} PASS ${END_COLOR} $function_name\n"
        fi
    fi
done

echo

if [ "$UPDATE_SNAPSHOTS" == "true" ]; then
    echo "✨ Snapshots were updated for all functions."
    echo
    exit 0
fi

if [ ${#functions_to_skip[@]} -gt 0 ]; then
    echo "🟨 The following function(s) were skipped:"
    for function_name in "${functions_to_skip[@]}"; do
        echo "- $function_name"
    done
    echo
fi

if [ ${#failed_functions[@]} -gt 0 ]; then
    echo "❌ The following function(s) did not match their snapshots:"
    for function_name in "${failed_functions[@]}"; do
        echo "- $function_name"
    done
    echo
    exit 1
fi

echo "✨ No difference found between snapshots and logs."
echo