#!/usr/bin/env sh
set -e

if [ "${DO_NOT_START_PROFILER}" = "1" ]; then
    echo "Skipping profiler start"
    echo "To start the profiler, run: launch.sh"
    sleep infinity
else
    mkdir -p bin/host-profiler
    go build -tags "remove_all_sd" -ldflags="-X github.com/DataDog/datadog-agent/pkg/version.AgentVersion=docker-dev" \
      -o bin/host-profiler/host-profiler ./cmd/host-profiler

    # Launch the profiler
    exec ./tools/host-profiler/launch.sh
fi
