#!/usr/bin/env sh
set -e

DD_API_KEY="$(sudo cat /run/secrets/dd-api-key)"
DD_APP_KEY="$(sudo cat /run/secrets/dd-app-key)"
export DD_API_KEY DD_APP_KEY

sudo mountpoint -q /sys/kernel/tracing || sudo mount -t tracefs tracefs /sys/kernel/tracing

cd /app

# Run the profiler (uses localhost for agent connection via shared network namespace)
# IPC artifacts (auth_token, ipc_cert.pem) are in /etc/datadog-agent from shared volume
sudo -E ./bin/full-host-profiler/full-host-profiler run \
  -c cmd/host-profiler/dist/host-profiler-config.yaml \
  --core-config /etc/datadog-agent/datadog.yaml
