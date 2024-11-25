#!/bin/bash

mkdir -p /tmp/otel-ci
cp ./test/otel/testdata/builder-config.yaml /tmp/otel-ci/
cp ./test/otel/testdata/collector-config.yaml /tmp/otel-ci/
cp ./tools/ci/retry.sh /tmp/otel-ci/
chmod +x /tmp/otel-ci/retry.sh

# TODO: Pin OCB to v0.114.0 once we upgrade collector dependencies to v0.114.0
# OCB_VERSION="0.113.0"
# CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" go.opentelemetry.io/collector/cmd/builder@v${OCB_VERSION}
# mv "$(go env GOPATH)/bin/builder" /tmp/otel-ci/ocb

# TODO: remove this once we upgrade collector dependencies to v0.114.0
# https://datadoghq.atlassian.net/browse/OTEL-2256
# Needs new package version published of logsagentexporter that uses NewLogs function
# clone collector repo and build cmd/builder from source
git clone https://github.com/open-telemetry/opentelemetry-collector.git /tmp/otel-ci/opentelemetry-collector
cwd=$(pwd)
cd "/tmp/otel-ci/opentelemetry-collector/cmd/builder" || (echo "failed to change to ocb source dir" && exit 1)
git checkout 1d87709aeabf492fdfd59ee110f8396c0441206b # pin to specific commit since APIs changed in v0.114.0
CGO_ENABLED=0 go build -o /tmp/otel-ci/ocb -trimpath -ldflags "-s -w" . > "${cwd}/go-install-ocb.log" 2>&1 || (echo "failed to build ocb" && exit 1)
cd "$cwd" || (echo "failed to change back to original dir" && exit 1)

chmod +x /tmp/otel-ci/ocb

/tmp/otel-ci/ocb --config=/tmp/otel-ci/builder-config.yaml > ocb-output.log 2>&1
grep -q 'Compiled' ocb-output.log || (echo "OCB failed to build custom collector" && exit 1)
grep -q '{"binary": "/tmp/otel-ci/otelcol-custom/otelcol-custom"}' ocb-output.log || (echo "OCB failed to build custom collector" && exit 1)

/tmp/otel-ci/otelcol-custom/otelcol-custom --config /tmp/otel-ci/collector-config.yaml > otelcol-custom.log 2>&1 &
OTELCOL_PID=$!
/tmp/otel-ci/retry.sh grep -q 'Everything is ready. Begin running and processing data.' otelcol-custom.log || (echo "Failed to start otelcol-custom" && kill $OTELCOL_PID && exit 1)

/tmp/otel-ci/retry.sh curl -k https://localhost:7777 > flare-info.log 2>&1 
grep -q '"provided_configuration": ""' flare-info.log || (echo "provided config is not empty" && kill $OTELCOL_PID && exit 1)
grep -q 'ddflare/dd-autoconfigured' flare-info.log || (echo "ddflare extension should be enabled" && kill $OTELCOL_PID && exit 1)
grep -q 'health_check/dd-autoconfigured' flare-info.log || (echo "health_check extension should be enabled" && kill $OTELCOL_PID && exit 1)
grep -q 'pprof/dd-autoconfigured' flare-info.log || (echo "pprof extension should be enabled" && kill $OTELCOL_PID && exit 1)
grep -q 'zpages/dd-autoconfigured' flare-info.log || (echo "zpages extension should be enabled" && kill $OTELCOL_PID && exit 1)
kill $OTELCOL_PID
echo "OCB build script completed successfully"
