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
# DOTNET_TRACE_LAYER_VERSION [number] - A specific layer version of dd-trace-dotnet to use.
# ENABLE_RACE_DETECTION [true|false] - Enables go race detection for the lambda extension
# ARCHITECTURE [arm64|amd64] - Specify the architecture to test. The default is amd64

DEFAULT_NODE_LAYER_VERSION=67
DEFAULT_PYTHON_LAYER_VERSION=50
DEFAULT_JAVA_TRACE_LAYER_VERSION=4
DEFAULT_DOTNET_TRACE_LAYER_VERSION=3
DEFAULT_ARCHITECTURE=amd64

# Text formatting constants
RED="\e[1;41m"
GREEN="\e[1;42m"
YELLOW="\e[1;43m"
MAGENTA="\e[1;45m"
END_COLOR="\e[0m"

set -e

script_utc_start_time=$(date -u +"%Y%m%dT%H%M%S")

if [ -z "$AWS_SECRET_ACCESS_KEY" || -z "$AWS_PROFILE" ]; then
    echo "No AWS credentials were found in the environment."
    echo "Note that only Datadog employees can run these integration tests."
    echo "Exiting without running tests..."

    # If credentials are not available, the run is considered a success
    # so as not to break CI for external users that don't have access to GitHub secrets
    exit 0
fi

if [ -z "$ARCHITECTURE" ]; then
    export ARCHITECTURE=$DEFAULT_ARCHITECTURE
fi

# Move into the root directory, so this script can be called from any directory
SERVERLESS_INTEGRATION_TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

cd "$SERVERLESS_INTEGRATION_TESTS_DIR/../../.."
LAMBDA_EXTENSION_REPOSITORY_PATH="../datadog-lambda-extension"
if [ "$BUILD_EXTENSION" != "false" ]; then
    echo "Building extension"

    if [ "$ENABLE_RACE_DETECTION" != "true" ]; then
        ENABLE_RACE_DETECTION=false
    fi

    # This version number is arbitrary and won't be used by AWS
    PLACEHOLDER_EXTENSION_VERSION=123

    ARCHITECTURE=$ARCHITECTURE RACE_DETECTION_ENABLED=$ENABLE_RACE_DETECTION VERSION=$PLACEHOLDER_EXTENSION_VERSION $LAMBDA_EXTENSION_REPOSITORY_PATH/scripts/build_binary_and_layer_dockerized.sh
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

if [ -z "$DOTNET_TRACE_LAYER_VERSION" ]; then
    export DOTNET_TRACE_LAYER_VERSION=$DEFAULT_DOTNET_TRACE_LAYER_VERSION
fi

echo "Testing for $ARCHITECTURE architecture"

echo "Using dd-lambda-js layer version: $NODE_LAYER_VERSION"
echo "Using dd-lambda-python version: $PYTHON_LAYER_VERSION"
echo "Using dd-trace-java layer version: $JAVA_TRACE_LAYER_VERSION"
echo "Using dd-trace-dotnet layer version: $DOTNET_TRACE_LAYER_VERSION"

# random 8-character ID to avoid collisions with other runs
stage=$(xxd -l 4 -c 4 -p </dev/random)

function remove_stack() {
    echo "Removing stack"
    serverless remove --stage "${stage}"
}

# always remove the stack before exiting, no matter what
trap remove_stack EXIT

# deploy the stack
serverless deploy --stage "${stage}"

metric_functions=(
    "metric-node"
    "metric-python"
    "metric-java"
    "metric-go"
    "metric-csharp"
    "metric-proxy"
    "timeout-node"
    "timeout-python"
    "timeout-java"
    "timeout-go"
    "timeout-csharp"
    "timeout-proxy"
    "error-node"
    "error-python"
    "error-java"
    "error-csharp"
    "error-proxy"
)
log_functions=(
    "log-node"
    "log-python"
    "log-java"
    "log-go"
    "log-csharp"
    "log-proxy"
)
trace_functions=(
    "trace-node"
    "trace-python"
    "trace-java"
    "trace-go"
    "trace-csharp"
    "trace-proxy"
)

all_functions=("${metric_functions[@]}" "${log_functions[@]}" "${trace_functions[@]}")

# Add a function to this list to skip checking its results
# This should only be used temporarily while we investigate and fix the test
functions_to_skip=(
    # Tagging behavior after a timeout is currently known to be flaky
    "timeout-node"
    "timeout-python"
    "timeout-java"
    "timeout-go"
    "timeout-csharp"
    "timeout-proxy"
    "trace-csharp" # Will be reactivated when the new dotnet layer will be released
    "trace-proxy" # Will be reactivated when sampling with proxy will be implemented
    "error-proxy"
    "log-proxy"
    "metric-proxy"
)

echo "Invoking functions for the first time..."
set +e # Don't exit this script if an invocation fails or there's a diff
for function_name in "${all_functions[@]}"; do
    serverless invoke --stage "${stage}" -f "${function_name}" &>/dev/null &
done
wait

# wait to make sure metrics aren't merged into a single metric
SECONDS_BETWEEN_INVOCATIONS=30
echo "Waiting $SECONDS_BETWEEN_INVOCATIONS seconds before invoking functions for the second time..."
sleep $SECONDS_BETWEEN_INVOCATIONS

# two invocations are needed since enhanced metrics are computed with the REPORT log line (which is created at the end of the first invocation)
echo "Invoking functions for the second time..."
for function_name in "${all_functions[@]}"; do
    serverless invoke --stage "${stage}" -f "${function_name}" -d '{"body": "testing request payload"}' &>/dev/null &
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
        raw_logs=$(serverless logs --stage "${stage}" -f "$function_name" --startTime "$script_utc_start_time")
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
                grep -v "BEGINLOG.*" |
                grep -v "BEGINTRACE.*" |
                grep "BEGINMETRIC.*" |
                perl -p -e "s/BEGINMETRIC/\1/g" |
                perl -p -e "s/ENDMETRIC/\1/g" |
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
                perl -p -e "s/(architecture:)(x86_64|arm64)/\1XXX/g" |
                perl -p -e "s/$stage/XXXXXX/g" |
                perl -p -e "s/[ ]$//g" |
                node parse-json.js
        )
    elif [[ " ${log_functions[*]} " =~ " ${function_name} " ]]; then
        # Normalize logs
        logs=$(
            echo "$raw_logs" |
                grep "BEGINLOG" |
                grep -v "BEGINMETRIC.*" |
                grep -v "BEGINTRACE.*" |
                perl -p -e "s/BEGINLOG/\1/g" |
                perl -p -e "s/ENDLOG/\1/g" |
                perl -p -e "s/(\"timestamp\": )\d{13}/\1\"XXX\"/g" |
                perl -p -e "s/(\"timestamp\": )\d{13}/\1\"XXX\"/g" |
                perl -p -e "s/\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}:\d{3}/TIMESTAMP/g" |
                perl -p -e "s/\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z/TIMESTAMP/g" |
                perl -p -e "s/\d{4}\/\d{2}\/\d{2}\s\d{2}:\d{2}:\d{2}/TIMESTAMP/g" |
                perl -p -e "s/\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}/TIMESTAMP/g" |
                perl -p -e "s/\"timestamp\":\d{13},/\1/g" |
                perl -p -e "s/([a-zA-Z0-9]{8}\-[a-zA-Z0-9]{4}\-[a-zA-Z0-9]{4}\-[a-zA-Z0-9]{4}\-[a-zA-Z0-9]{12})/\0\XXX/g" |
                perl -p -e "s/$stage/STAGE/g" |
                perl -p -e "s/(architecture:)(x86_64|arm64)/\1XXX/g" |
                # ignore a Lambda error that occurs sporadically for log-csharp
                # see here for more info: https://repost.aws/questions/QUq2OfIFUNTCyCKsChfJLr5w/lambda-function-working-locally-but-crashing-on-aws
                perl -n -e "print unless /LAMBDA_RUNTIME Failed to get next invocation. No Response from endpoint/ or \
                 /An error occurred while attempting to execute your code.: LambdaException/ or \
                 /terminate called after throwing an instance of 'std::logic_error'/ or \
                 /basic_string::_M_construct null not valid/" |
                node parse-json.js
        )
    else
        # Normalize traces
        logs=$(
            echo "$raw_logs" |
                grep "BEGINTRACE" |
                grep -v "BEGINMETRIC.*" |
                grep -v "BEGINLOG.*" |
                perl -p -e "s/BEGINTRACE/\1/g" |
                perl -p -e "s/ENDTRACE/\1/g" |
                perl -p -e "s/(ts\":)[0-9]{10}/\1XXX/g" |
                perl -p -e "s/((startTime|endTime|traceID|trace_id|span_id|parent_id|start|system.pid)\":)[0-9]+/\1null/g" |
                perl -p -e "s/((tracer_version|language_version)\":)[\"a-zA-Z0-9~\-\.\_]+/\1null/g" |
                perl -p -e "s/(duration\":)[0-9]+/\1null/g" |
                perl -p -e "s/((datadog_lambda|dd_trace)\":\")[0-9]+\.[0-9]+\.[0-9]+/\1X\.X\.X/g" |
                perl -p -e "s/(,\"request_id\":\")[a-zA-Z0-9\-,]+\"/\1null\"/g" |
                perl -p -e "s/(,\"runtime-id\":\")[a-zA-Z0-9\-,]+\"/\1null\"/g" |
                perl -p -e "s/(,\"system.pid\":\")[a-zA-Z0-9\-,]+\"/\1null\"/g" |
                perl -p -e "s/(\"_dd.no_p_sr\":)[0-9\.]+/\1null/g" |
                perl -p -e "s/(\"architecture\":)\"(x86_64|arm64)\"/\1\"XXX\"/g" |
                perl -p -e "s/(\"process_id\":)[0-9]+/\1null/g" |
                perl -p -e "s/$stage/XXXXXX/g" |
                perl -p -e "s/[ ]$//g" |
                node parse-json.js
        )
    fi

    function_snapshot_path="./snapshots/${function_name}"

    jsonLogs="$(echo $logs | node parse-json.js)"

    if [ ! -f "$function_snapshot_path" ]; then
        printf "${MAGENTA} CREATE ${END_COLOR} $function_name\n"
        echo "$jsonLogs" >"$function_snapshot_path"
    elif [ "$UPDATE_SNAPSHOTS" == "true" ]; then
        printf "${MAGENTA} UPDATE ${END_COLOR} $function_name\n"
        echo "$jsonLogs" > "$function_snapshot_path"
    else
        if [[ " ${functions_to_skip[*]} " =~ " ${function_name} " ]]; then
            printf "${YELLOW} SKIP ${END_COLOR} $function_name\n"
            continue
        fi
        # Skip all csharp tests when on arm64, .NET is not supported at all for arm architecture
        if [[ $ARCHITECTURE == "arm64" && $function_name =~ "csharp" ]]; then
            printf "${YELLOW} SKIP ${END_COLOR} $function_name, no .NET support on arm64\n"
            continue
        fi
        diff_output=$(echo "$jsonLogs" | diff - "$function_snapshot_path")
        if [ $? -eq 1 ]; then
            failed_functions+=("$function_name")

            echo
            printf "${RED} FAIL ${END_COLOR} $function_name\n"
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
