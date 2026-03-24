#!/usr/bin/env sh
set -e

if [ "${DO_NOT_START_PROFILER}" = "1" ]; then
    echo "Skipping profiler start"
    echo "To start the profiler, run: launch.sh"
    sleep infinity
else
    # Build full-host-profiler
    mkdir -p bin/full-host-profiler
    go build -ldflags="-X github.com/DataDog/datadog-agent/pkg/version.AgentVersion=docker-dev" \
      -o bin/full-host-profiler/full-host-profiler ./cmd/host-profiler

    # Launch the profiler
    exec ./tools/host-profiler/launch.sh
fi
