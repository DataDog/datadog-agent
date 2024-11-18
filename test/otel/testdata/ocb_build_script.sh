#!/bin/bash

mkdir -p /tmp/otel-ci
cp ./test/otel/testdata/builder-config.yaml /tmp/otel-ci/
cp ./test/otel/testdata/collector-config.yaml /tmp/otel-ci/

# TODO: Pin OCB to v0.114.0 once we upgrade collector dependencies to v0.114.0
# OCB_VERSION="0.113.0"
# CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" go.opentelemetry.io/collector/cmd/builder@v${OCB_VERSION}
# mv "$(go env GOPATH)/bin/builder" /tmp/otel-ci/ocb

# TODO: remove this once we upgrade collector dependencies to v0.114.0
# clone collector repo and build cmd/builder from source
git clone --depth 1 https://github.com/open-telemetry/opentelemetry-collector.git /tmp/otel-ci/opentelemetry-collector
cwd=$(pwd)
cd "/tmp/otel-ci/opentelemetry-collector/cmd/builder" || (echo "failed to change to ocb source dir" && exit 1)
CGO_ENABLED=0 go build -o /tmp/otel-ci/ocb -trimpath -ldflags "-s -w" . > "${cwd}/go-install-ocb.log" 2>&1 || (echo "failed to build ocb" && exit 1)
cd "$cwd" || (echo "failed to change back to original dir" && exit 1)

chmod +x /tmp/otel-ci/ocb

/tmp/otel-ci/ocb --config=/tmp/otel-ci/builder-config.yaml > ocb-output.log 2>&1
grep -q 'Compiled' ocb-output.log || (echo "OCB failed to build custom collector" && exit 1)
grep -q '{"binary": "/tmp/otel-ci/otelcol-custom/otelcol-custom"}' ocb-output.log || (echo "OCB failed to build custom collector" && exit 1)

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
grep -q 'ddflare/dd-autoconfigured' flare-info.log || (echo "ddflare extension should be enabled" && kill $OTELCOL_PID && exit 1)
grep -q 'health_check/dd-autoconfigured' flare-info.log || (echo "health_check extension should be enabled" && kill $OTELCOL_PID && exit 1)
grep -q 'pprof/dd-autoconfigured' flare-info.log || (echo "pprof extension should be enabled" && kill $OTELCOL_PID && exit 1)
grep -q 'zpages/dd-autoconfigured' flare-info.log || (echo "zpages extension should be enabled" && kill $OTELCOL_PID && exit 1)
kill $OTELCOL_PID
echo "OCB build script completed successfully"
