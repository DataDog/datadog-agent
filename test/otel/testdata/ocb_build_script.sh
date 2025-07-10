#!/bin/bash
set -eo pipefail

OTELCOL_PID=0
KEEP_TEMP=0

cleanup() {
	# Only kill collector if PID was set
	if [[ $OTELCOL_PID -ne 0 ]]; then
		kill "$OTELCOL_PID" 2>/dev/null || true
	fi

	# Conditionally remove temp directory
	if [[ $KEEP_TEMP -eq 0 ]]; then
		rm -rf /tmp/otel-ci
	fi
}

usage() {
	echo "Usage: $0 [OPTIONS]"
	echo "Build and test OTel Collector configuration"
	echo ""
	echo "Options:"
	echo "  -k, --keep-temp  Keep temporary files in /tmp/otel-ci after completion"
	echo "  -h, --help       Show this help message"
	exit 0
}

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
	case "$1" in
	-k | --keep-temp)
		KEEP_TEMP=1
		shift
		;;
	-h | --help)
		usage
		;;
	*)
		echo "Invalid option: $1" >&2
		exit 1
		;;
	esac
done

# Setup cleanup trap
trap cleanup EXIT

# Create working directory
WORK_DIR="/tmp/otel-ci"
mkdir -p "$WORK_DIR"

# Copy configuration files
current_dir=$(pwd)
cp -v ./test/otel/testdata/builder-config.yaml "$WORK_DIR/"
cp -v ./test/otel/testdata/collector-config.yaml "$WORK_DIR/"
cp -v ./tools/ci/retry.sh "$WORK_DIR/retry.sh"
chmod +x "$WORK_DIR/retry.sh"

dd_mods=$(find . -type f -name "go.mod" -exec dirname {} \; | sort | sed 's/.//')

# Generate module replacements
{
	echo "replaces:"
	for mod in $dd_mods; do
		echo "- github.com/DataDog/datadog-agent$mod => $current_dir$mod"
	done
} >>"$WORK_DIR/builder-config.yaml"

# Install and configure OCB
OCB_VERSION="0.129.0"
CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" \
	go.opentelemetry.io/collector/cmd/builder@v${OCB_VERSION}
mv -v "$(go env GOPATH)/bin/builder" "$WORK_DIR/ocb"
chmod +x "$WORK_DIR/ocb"

# Build collector
echo "Building OTel Collector..."
if ! "$WORK_DIR/ocb" --config="$WORK_DIR/builder-config.yaml" >ocb-output.log 2>&1; then
	echo "OCB build failed with exit code $?" >&2
	exit 1
fi

# Verify build output
required_strings=(
	'Compiled'
	'{"binary": "/tmp/otel-ci/otelcol-custom/otelcol-custom"}'
)
for s in "${required_strings[@]}"; do
	if ! grep -q "$s" ocb-output.log; then
		echo "Missing required build output: $s" >&2
		exit 1
	fi
done

# Start collector and verify operation
echo "Starting Collector..."
"$WORK_DIR/otelcol-custom/otelcol-custom" --config "$WORK_DIR/collector-config.yaml" >otelcol-custom.log 2>&1 &
OTELCOL_PID=$!

if ! "$WORK_DIR/retry.sh" grep -q 'Everything is ready. Begin running and processing data.' otelcol-custom.log; then
	echo "Collector failed to start properly" >&2
	exit 1
fi

# Verify endpoint responses
echo "Validating endpoints..."
required_patterns=(
	'"provided_configuration": ""'
	'ddflare/dd-autoconfigured'
	'health_check/dd-autoconfigured'
	'pprof/dd-autoconfigured'
	'zpages/dd-autoconfigured'
	'infraattributes'
)

if ! "$WORK_DIR/retry.sh" curl -k https://localhost:7777 >flare-info.log 2>&1; then
	echo "Failed to query flare endpoint" >&2
	exit 1
fi

for pattern in "${required_patterns[@]}"; do
	if ! grep -q "$pattern" flare-info.log; then
		echo "Missing required pattern in response: $pattern" >&2
		exit 1
	fi
done

echo "OCB build script completed successfully"
