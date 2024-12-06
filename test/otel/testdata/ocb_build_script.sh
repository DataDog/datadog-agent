#!/bin/bash
set -e
OTELCOL_PID=0

mkdir -p /tmp/otel-ci
trap 'rm -rf /tmp/otel-ci && kill $OTELCOL_PID' EXIT
cp ./test/otel/testdata/builder-config.yaml /tmp/otel-ci/
cp ./test/otel/testdata/collector-config.yaml /tmp/otel-ci/
cp ./tools/ci/retry.sh /tmp/otel-ci/
chmod +x /tmp/otel-ci/retry.sh

OCB_VERSION="0.114.0"
CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" go.opentelemetry.io/collector/cmd/builder@v${OCB_VERSION}
mv "$(go env GOPATH)/bin/builder" /tmp/otel-ci/ocb

chmod +x /tmp/otel-ci/ocb

/tmp/otel-ci/ocb --config=/tmp/otel-ci/builder-config.yaml > ocb-output.log 2>&1
grep -q 'Compiled' ocb-output.log || (echo "OCB failed to build custom collector" && exit 1)
grep -q '{"binary": "/tmp/otel-ci/otelcol-custom/otelcol-custom"}' ocb-output.log || (echo "OCB failed to build custom collector" && exit 1)

/tmp/otel-ci/otelcol-custom/otelcol-custom --config /tmp/otel-ci/collector-config.yaml > otelcol-custom.log 2>&1 &
OTELCOL_PID=$!
/tmp/otel-ci/retry.sh grep -q 'Everything is ready. Begin running and processing data.' otelcol-custom.log || (echo "Failed to start otelcol-custom" && exit 1)

/tmp/otel-ci/retry.sh curl -k https://localhost:7777 > flare-info.log 2>&1 
grep -q '"provided_configuration": ""' flare-info.log || (echo "provided config is not empty" && exit 1)
grep -q 'ddflare/dd-autoconfigured' flare-info.log || (echo "ddflare extension should be enabled" && exit 1)
grep -q 'health_check/dd-autoconfigured' flare-info.log || (echo "health_check extension should be enabled" && exit 1)
grep -q 'pprof/dd-autoconfigured' flare-info.log || (echo "pprof extension should be enabled" && exit 1)
grep -q 'zpages/dd-autoconfigured' flare-info.log || (echo "zpages extension should be enabled" && exit 1)

echo "OCB build script completed successfully"
