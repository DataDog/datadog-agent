#!/bin/bash

# Runs Datadog Lambda Extension integration tests.
# NOTE: Use aws-vault clear before running tests to ensure credentials do not expire during a test run

# Run tests:
#   aws-vault clear && aws-vault exec serverless-sandbox-account-admin -- ./run.sh [suite]
# Regenerate snapshots:
#   aws-vault clear && UPDATE_SNAPSHOTS=true aws-vault exec serverless-sandbox-account-admin -- ./run.sh [suite]

# Optionally specify a [suite] to limit tests executed to a specific group (all tests are run if none is provided).
# Valid values are:
#   - metric
#   - log
#   - trace
#   - appsec

# Optional environment variables:

# UPDATE_SNAPSHOTS [true|false] - Use this when you want to update snapshots instead of running tests. The default is false.
# BUILD_EXTENSION [true|false] - Whether to build the extension or re-use the previous build. The default is true.
# NODE_LAYER_VERSION [number] - A specific layer version of datadog-lambda-js to use.
# PYTHON_LAYER_VERSION [number] - A specific layer version of datadog-lambda-py to use.
# JAVA_TRACE_LAYER_VERSION [number] - A specific layer version of dd-trace-java to use.
# DOTNET_TRACE_LAYER_VERSION [number] - A specific layer version of dd-trace-dotnet to use.
# ENABLE_RACE_DETECTION [true|false] - Enables go race detection for the lambda extension
# ARCHITECTURE [arm64|amd64] - Specify the architecture to test. The default is amd64

DEFAULT_NODE_LAYER_VERSION=99
DEFAULT_PYTHON_LAYER_VERSION=77
DEFAULT_JAVA_TRACE_LAYER_VERSION=11
DEFAULT_DOTNET_TRACE_LAYER_VERSION=9
DEFAULT_ARCHITECTURE=amd64

# Text formatting constants
RED="\e[1;41m"
GREEN="\e[1;42m"
YELLOW="\e[1;43m"
MAGENTA="\e[1;45m"
END_COLOR="\e[0m"

set -e

script_utc_start_time=$(date -u +"%Y%m%dT%H%M%S")

if [ -z "$AWS_SECRET_ACCESS_KEY" ] && [ -z "$AWS_PROFILE" ]; then
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
LAMBDA_EXTENSION_REPOSITORY_PATH="${LAMBDA_EXTENSION_REPOSITORY_PATH:-../datadog-lambda-extension}"
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

export NODE_LAYER_VERSION=${NODE_LAYER_VERSION:-$DEFAULT_NODE_LAYER_VERSION}
export PYTHON_LAYER_VERSION=${PYTHON_LAYER_VERSION:-$DEFAULT_PYTHON_LAYER_VERSION}
export JAVA_TRACE_LAYER_VERSION=${JAVA_TRACE_LAYER_VERSION:-$DEFAULT_JAVA_TRACE_LAYER_VERSION}
export DOTNET_TRACE_LAYER_VERSION=${DOTNET_TRACE_LAYER_VERSION:-$DEFAULT_DOTNET_TRACE_LAYER_VERSION}

echo "Testing for $ARCHITECTURE architecture"

echo "Using dd-lambda-js layer version: $NODE_LAYER_VERSION"
echo "Using dd-lambda-python version: $PYTHON_LAYER_VERSION"
echo "Using dd-trace-java layer version: $JAVA_TRACE_LAYER_VERSION"
echo "Using dd-trace-dotnet layer version: $DOTNET_TRACE_LAYER_VERSION"

# random 8-character ID to avoid collisions with other runs
stage=$(xxd -l 4 -c 4 -p </dev/random)

# Default to AWS Account 425362996713 because the workflow runs w/o sts:GetCallerIdentity permissions
aws_account=$(aws sts get-caller-identity --query 'Account' --output text 2>/dev/null || echo '425362996713')

function remove_stack() {
    echo "Removing stack"
    serverless remove --stage "${stage}"
}

# always remove the stack before exiting, no matter what
trap remove_stack EXIT

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
    "otlp-python"
)
appsec_functions=(
    "appsec-node"
    "appsec-python"
    "appsec-java"
    "appsec-go"
    "appsec-csharp"
)
proxy_functions=(
    "proxy-env-apikey"
    "proxy-yaml-apikey"
    "proxy-yaml-env-apikey"
    "proxy-env-secret"
    "proxy-yaml-secret"
    "proxy-yaml-env-secret"
    "proxy-env-kms"
    "proxy-yaml-kms"
    "proxy-yaml-env-kms"
)

declare -a all_functions # This is an array
if [ $# == 1 ]; then
    case $1 in
        metric)
            all_functions=("${metric_functions[@]}")
        ;;
        log)
            all_functions=("${log_functions[@]}")
        ;;
        trace)
            all_functions=("${trace_functions[@]}")
        ;;
        appsec)
            all_functions=("${appsec_functions[@]}")
        ;;
        proxy)
            all_functions=("${proxy_functions[@]}")
        ;;
        *)
            echo "Unknown test suite: '$1' (valid names are: metric, log, trace, appsec, proxy)"
            exit 1
        ;;
    esac
    echo "Selected test suite '$1' contains ${#all_functions[@]} functions..."
else
    all_functions=("${metric_functions[@]}" "${log_functions[@]}" "${trace_functions[@]}" "${appsec_functions[@]}" "${proxy_functions[@]}")
fi

# Set feature environment for the Serverless stack to avoid deploying useless stuff...
export RUN_SUITE_METRIC=false
export RUN_SUITE_TIMEOUT=false
export RUN_SUITE_ERROR=false
export RUN_SUITE_LOG=false
export RUN_SUITE_TRACE=false
export RUN_SUITE_OTLP=false
export RUN_SUITE_APPSEC=false
export RUN_SUITE_PROXY=false
for function_name in "${all_functions[@]}"; do
    case $function_name in
    metric-*)
        export RUN_SUITE_METRIC=true
    ;;
    timeout-*)
        export RUN_SUITE_TIMEOUT=true
    ;;
    error-*)
        export RUN_SUITE_ERROR=true
    ;;
    log-*)
        export RUN_SUITE_LOG=true
    ;;
    trace-*)
        export RUN_SUITE_TRACE=true
    ;;
    otlp-*)
        export RUN_SUITE_OTLP=true
    ;;
    appsec-*)
        export RUN_SUITE_APPSEC=true
    ;;
    proxy-*)
        export RUN_SUITE_PROXY=true
    ;;
    *)
        echo "⚠️ Un-mapped test function: ${function_name}, the necessary components may not be deployed!"
        ;;
    esac
done

# a bug in opentelemetry instrumentation makes it impossible to define a
# handler inside of a directory
# see https://github.com/open-telemetry/opentelemetry-lambda/issues/655
cp $SERVERLESS_INTEGRATION_TESTS_DIR/src/otlpPython.py $SERVERLESS_INTEGRATION_TESTS_DIR/otlpPython.py

# deploy the stack
(cd ${SERVERLESS_INTEGRATION_TESTS_DIR}; npm install --no-save serverless-plugin-conditional-functions)
serverless deploy --stage "${stage}"

# deploy proxy functions with a different datadog.yaml
if [ "$RUN_SUITE_PROXY" = true ]; then
    echo "Updating datadog.yaml for proxy tests..."

    mv $SERVERLESS_INTEGRATION_TESTS_DIR/datadog.yaml $SERVERLESS_INTEGRATION_TESTS_DIR/datadog-temp.yaml
    mv $SERVERLESS_INTEGRATION_TESTS_DIR/datadog-proxy.yaml $SERVERLESS_INTEGRATION_TESTS_DIR/datadog.yaml

    for function_name in "${proxy_functions[@]}"; do
        if [[ "$function_name" = *-yaml-* ]]; then
            serverless deploy function --stage "${stage}" --function $function_name
        fi
    done

    mv $SERVERLESS_INTEGRATION_TESTS_DIR/datadog.yaml $SERVERLESS_INTEGRATION_TESTS_DIR/datadog-proxy.yaml
    mv $SERVERLESS_INTEGRATION_TESTS_DIR/datadog-temp.yaml $SERVERLESS_INTEGRATION_TESTS_DIR/datadog.yaml
fi

rm $SERVERLESS_INTEGRATION_TESTS_DIR/otlpPython.py

# Add a function to this list to skip checking its results
# This should only be used temporarily while we investigate and fix the test
functions_to_skip=()

echo "Invoking functions for the first time..."
set +e # Don't exit this script if an invocation fails or there's a diff
for function_name in "${all_functions[@]}"; do
    case $function_name in
    appsec-*)
        # Invoke appsec functions with an arbitrary attack tool user-agent to trigger a known WAF rule
        serverless invoke --stage "${stage}" -f "${function_name}" -d "$(cat appsec-payload.json)" &>/dev/null &
        ;;
    *)
        serverless invoke --stage "${stage}" -f "${function_name}" &>/dev/null &
        ;;
    esac
done
wait

# wait to make sure metrics aren't merged into a single metric
SECONDS_BETWEEN_INVOCATIONS=30
echo "Waiting $SECONDS_BETWEEN_INVOCATIONS seconds before invoking functions for the second time..."
sleep $SECONDS_BETWEEN_INVOCATIONS

# two invocations are needed since enhanced metrics are computed with the REPORT log line (which is created at the end of the first invocation)
echo "Invoking functions for the second time..."
for function_name in "${all_functions[@]}"; do
    case $function_name in
    appsec-*)
        # Invoke appsec functions with an arbitrary attack tool user-agent to trigger a known WAF rule
        serverless invoke --stage "${stage}" -f "${function_name}" -d "$(cat appsec-payload.json)" &>/dev/null &
        ;;
    *)
        serverless invoke --stage "${stage}" -f "${function_name}" -d '{"body": "testing request payload"}' &>/dev/null &
        ;;
    esac
done
wait

failed_functions=()

if [ -z $RAWLOGS_DIR ]; then
    RAWLOGS_DIR=$(mktemp -d)
else
    mkdir -p $RAWLOGS_DIR
fi
echo "Raw logs will be written to ${RAWLOGS_DIR}"

for function_name in "${all_functions[@]}"; do
    echo "Fetching logs for ${function_name}..."
    retry_counter=1
    while [ $retry_counter -lt 11 ]; do
        raw_logs=$(serverless logs --stage "${stage}" -f "$function_name" --startTime "$script_utc_start_time")
        fetch_logs_exit_code=$?
        if [ $fetch_logs_exit_code -eq 1 ]; then
            printf "\e[A\e[K" # erase previous log line
            echo "Retrying fetch logs for $function_name in 10 seconds... (retry #$retry_counter)"
            retry_counter=$(($retry_counter + 1))
            sleep 10
            continue
        fi
        if [[ "${function_name}" = timeout-* || "${function_name}" = proxy-* ]]; then
            echo "Ignoring Lambda report check count as this is a timeout or proxy example..."
        else
            count=$(echo $raw_logs | grep -o 'REPORT RequestId' | wc -l)
            if [ $count -lt 2 ]; then
                echo "Logs not done flushing yet ($count Lambda reports seen, at least 2 expected)..."
                echo "Retrying fetch logs for $function_name in 60 seconds... (retry #$retry_counter)"
                retry_counter=$(($retry_counter + 1))
                sleep 60
                continue
            fi
        fi
        break
    done
    printf "\e[A\e[K" # erase previous log line

    echo $raw_logs > $RAWLOGS_DIR/$function_name

    # Replace invocation-specific data like timestamps and IDs with XXX to normalize across executions
    if [[ " ${metric_functions[*]} " =~ " ${function_name} " ]]; then
        norm_type=metrics
    elif [[ " ${log_functions[*]} " =~ " ${function_name} " ]]; then
        norm_type=logs
    elif [[ " ${appsec_functions[*]} " =~ " ${function_name} " ]]; then
        norm_type=appsec
    elif [[ " ${trace_functions[*]} " =~ " ${function_name} " ]]; then
        norm_type=traces
    elif [[ " ${proxy_functions[*]} " =~ " ${function_name} " ]]; then
        norm_type=proxy
    fi
    logs=$(python3 log_normalize.py --accountid ${aws_account} --type $norm_type --logs "file:$RAWLOGS_DIR/$function_name" --stage $stage)

    function_snapshot_path="./snapshots/${function_name}"

    if [ ! -f "$function_snapshot_path" ]; then
        printf "${MAGENTA} CREATE ${END_COLOR} $function_name\n"
        echo "$logs" >"$function_snapshot_path"
    elif [ "$UPDATE_SNAPSHOTS" == "true" ]; then
        printf "${MAGENTA} UPDATE ${END_COLOR} $function_name\n"
        echo "$logs" > "$function_snapshot_path"
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
        diff_output=$(echo "$logs" | diff - "$function_snapshot_path")
        if [ $? -eq 1 ]; then
            failed_functions+=("$function_name")

            echo
            printf "${RED} FAIL ${END_COLOR} $function_name\n"
            echo "Diff:"
            echo
            echo "$diff_output"
            echo
            echo "Raw output can be found in $RAWLOGS_DIR/$function_name (or in CI output artifacts)"
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
    echo "+++ Need help with failures?  Check https://datadoghq.atlassian.net/l/cp/H7CdziU9 for a list of known issues and suggested next steps +++"
    echo
    exit 1
fi

echo "✨ No difference found between snapshots and logs."
echo
