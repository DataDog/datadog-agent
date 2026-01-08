#!/bin/sh

# Check if Alpine
if [ -f "/etc/alpine-release" ]; then
    echo "Alpine images are not supported for .NET automatic instrumentation with serverless-init"
    exit 1
fi

# Make sure curl and jq are installed
apt-get update && apt-get install -y curl jq

ghcurl() {
  if [ -n "$GITHUB_TOKEN" ]; then
    echo "Github token provided" >&2
    curl -sSL -w '{"status_code": %{http_code}}' -H "Authorization: Bearer $GITHUB_TOKEN" "$@" | jq -sre add
  else
    echo "Github token not provided" >&2
    curl -sSL -w '{"status_code": %{http_code}}' "$@" | jq -sre add
  fi
}

# Get latest released version of dd-trace-dotnet
response=$(ghcurl https://api.github.com/repos/DataDog/dd-trace-dotnet/releases/latest)
sanitized_response=$(echo "$response" | tr -d '\000-\037') # remove control characters from json response
echo "Status code of version request: $(echo "$sanitized_response" | jq '.status_code')"
TRACER_VERSION=$(echo "$sanitized_response" | jq -r '.tag_name // empty'  | sed 's/^v//')

if [ -z "$TRACER_VERSION" ]; then
  echo "Error: Could not determine the tracer version. Exiting." >&2
  exit 1
fi

# Download the tracer to the dd_tracer folder
echo Downloading version "${TRACER_VERSION}" of the .NET tracer into /tmp/datadog-dotnet-apm.tar.gz
response=$(ghcurl "https://github.com/DataDog/dd-trace-dotnet/releases/download/v${TRACER_VERSION}/datadog-dotnet-apm-${TRACER_VERSION}.tar.gz" \
    -o /tmp/datadog-dotnet-apm.tar.gz)
echo "Status code of download request: $(echo "$response" | jq '.status_code')"

# Unarchive the tracer and remove the tmp
mkdir -p /dd_tracer/dotnet
tar -xzf /tmp/datadog-dotnet-apm.tar.gz -C /dd_tracer/dotnet
rm /tmp/datadog-dotnet-apm.tar.gz

# Create Log path
/dd_tracer/dotnet/createLogPath.sh
