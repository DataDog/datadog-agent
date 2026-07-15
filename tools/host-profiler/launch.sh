#!/usr/bin/env sh
set -e

DD_API_KEY="$(sudo cat /run/secrets/dd-api-key)"
DD_APP_KEY="$(sudo cat /run/secrets/dd-app-key)"
export DD_API_KEY DD_APP_KEY

sudo mountpoint -q /sys/kernel/tracing || sudo mount -t tracefs tracefs /sys/kernel/tracing

cd /app

# HOST_PROFILER_CONFIG lets callers (e.g. docker-compose services) point at an alternate
# OTel config, such as host-profiler-config-dogtel.yaml, without duplicating this script.
# HOST_PROFILER_BIN_NAME must match the --output-name passed to build_and_launch.sh's
# `dda inv host-profiler.build`, so multiple variants stay distinguishable in `ps`/`top`.
sudo -E "./bin/host-profiler/${HOST_PROFILER_BIN_NAME:-host-profiler}" run \
  --config "${HOST_PROFILER_CONFIG:-cmd/host-profiler/dist/host-profiler-config.yaml}"
