#!/bin/bash

ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ]; then
    ARCH="arm64"
fi
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
OCB_URL="https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/cmd%2Fbuilder%2Fv0.113.0/ocb_0.113.0_${OS}_${ARCH}"
mkdir -p /tmp/otel-ci
cp ./test/otel/testdata/* /tmp/otel-ci/
wget -O /tmp/otel-ci/ocb "$OCB_URL"
chmod +x /tmp/otel-ci/ocb

/tmp/otel-ci/ocb --config=/tmp/otel-ci/builder-config.yaml > ocb-output.log 2>&1
grep -q 'Compiled' ocb-output.log || (echo "OCB failed to compile" && exit 1)
grep -q '{"binary": "/tmp/otel-ci/otelcol-custom/otelcol-custom"}' ocb-output.log || (echo "OCB failed to compile" && exit 1)

/tmp/otel-ci/otelcol-custom/otelcol-custom --config /tmp/otel-ci/collector-config.yaml > otelcol-custom.log 2>&1 &
OTELCOL_PID=$!

# Function to retry grep up to 5 times
retry_grep() {
    local phrase="$1"
    local file="$2"
    local retries=5
    local count=0

    until grep -q "$phrase" "$file"; do
        count=$((count + 1))
        if [ $count -ge $retries ]; then
            echo "Failed to find phrase '$phrase' in $file after $retries attempts"
            kill $OTELCOL_PID
            exit 1
        fi
        sleep 1
    done
}

# Function to retry curl up to 5 times
retry_curl() {
    local url="$1"
    local retries=5
    local count=0

    until curl -k "$url"; do
        count=$((count + 1))
        if [ $count -ge $retries ]; then
            echo "Failed to successfully curl '$url' after $retries attempts"
            kill $OTELCOL_PID
            exit 1
        fi
        sleep 1
    done
}

retry_grep 'Everything is ready. Begin running and processing data.' otelcol-custom.log

retry_curl https://localhost:7777 > flare-info.log 2>&1
grep -q '"provided_configuration": ""' flare-info.log || (echo "provided config is not empty" && kill $OTELCOL_PID && exit 1)
grep -q 'extensions:\\n  - ddflare\\n' flare-info.log || (echo "ddflare extension should be enabled" && kill $OTELCOL_PID && exit 1)
kill $OTELCOL_PID
echo "OCB build script completed successfully"