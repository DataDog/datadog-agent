#!/usr/bin/env sh
set -e

if [ "${DO_NOT_START_PROFILER}" = "1" ]; then
    echo "Skipping profiler start"
    echo "To start the profiler, run: launch.sh"
    sleep infinity
else
    dda inv host-profiler.build --output-name="${HOST_PROFILER_BIN_NAME:-host-profiler}"
    exec ./tools/host-profiler/launch.sh
fi
