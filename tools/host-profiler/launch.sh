#!/usr/bin/env sh
set -e

if [ "${DO_NOT_START_PROFILER}" = "1" ]; then
    echo "Skipping profiler start"
    sleep infinity
else
    # Build full-host-profiler
    mkdir -p bin/full-host-profiler
    go build -ldflags="-X github.com/DataDog/datadog-agent/pkg/version.AgentVersion=docker-dev" \
      -o bin/full-host-profiler/full-host-profiler ./cmd/host-profiler

    # Run the profiler (uses localhost for agent connection via shared network namespace)
    # IPC artifacts (auth_token, ipc_cert.pem) are in /etc/datadog-agent from shared volume
    sudo -E ./bin/full-host-profiler/full-host-profiler run \
      -c cmd/host-profiler/dist/host-profiler-config.yaml \
      --core-config /etc/datadog-agent/datadog.yaml
fi
