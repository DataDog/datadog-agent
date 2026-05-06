#!/bin/bash
# Manual reproducer for the discovery probe retry behaviour: starts the
# agent, then a krakend container whose entrypoint sleeps 60 s before
# exec'ing the binary. AD fires while the HTTP endpoint is unreachable;
# the retry loop must keep probing until the application is up.
#
# See docs/superpowers/2026-05-06-discover-e2e-smoke.md for context and
# expected output. Requires datadog/agent-dev:discovery-local already
# built (see ../README.md).
set -uo pipefail

REPRO_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
AGENT_REPO=$(cd "$REPRO_DIR/../../../.." && pwd)
INTEGRATIONS_CORE_REPO="${INTEGRATIONS_CORE_REPO:-$AGENT_REPO/../integrations-core}"
INTEGRATIONS_CORE_REPO=$(cd "$INTEGRATIONS_CORE_REPO" && pwd) || {
  echo "INTEGRATIONS_CORE_REPO does not point at a directory; set it explicitly." >&2
  exit 1
}
export INTEGRATIONS_CORE_REPO

AGENT_NAME=dd-agent-repro
COMPOSE="$REPRO_DIR/docker-compose.yml"
SITE_PACKAGES=/opt/datadog-agent/embedded/lib/python3.13/site-packages
CONF_D=/etc/datadog-agent/conf.d

# Clean slate
docker rm -f "$AGENT_NAME" >/dev/null 2>&1 || true
docker compose -f "$COMPOSE" -p krakend-delayed down --volumes >/dev/null 2>&1 || true

echo "=== t+0s: starting agent ==="
docker run -d --name "$AGENT_NAME" \
  --network host \
  -e DD_API_KEY=000001 \
  -e DD_HOSTNAME=krakend-delayed-repro \
  -e DD_LOG_LEVEL=debug \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /proc/:/host/proc/:ro \
  -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
  -v "$INTEGRATIONS_CORE_REPO/krakend/datadog_checks/krakend/data/auto_conf_discovery.yaml:$CONF_D/krakend.d/auto_conf_discovery.yaml:ro" \
  -v "$INTEGRATIONS_CORE_REPO/krakend/datadog_checks/krakend:$SITE_PACKAGES/datadog_checks/krakend:ro" \
  -v "$INTEGRATIONS_CORE_REPO/datadog_checks_base/datadog_checks/base/utils/discovery:$SITE_PACKAGES/datadog_checks/base/utils/discovery:ro" \
  -v "$INTEGRATIONS_CORE_REPO/datadog_checks_base/datadog_checks/base/checks/openmetrics/v2/base.py:$SITE_PACKAGES/datadog_checks/base/checks/openmetrics/v2/base.py:ro" \
  datadog/agent-dev:discovery-local >/dev/null

# Wait for agent to be up
for _ in {1..30}; do
  if docker exec "$AGENT_NAME" agent status >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
echo "agent up"
sleep 3

echo
echo "=== t+~5s: starting DELAYED krakend (60s sleep before listening on 9090) ==="
docker compose -f "$COMPOSE" -p krakend-delayed up -d --build 2>&1 | tail -3
KRAKEND_START_EPOCH=$(date +%s)
KRAKEND_LISTEN_EPOCH=$((KRAKEND_START_EPOCH + 60))

echo
echo "=== watching ==="
echo "t=$(date +%H:%M:%S) krakend container started; should listen on :9090 around $(date -d "@$KRAKEND_LISTEN_EPOCH" +%H:%M:%S)"

# Phase 1: 0-15s after krakend container start
# AD should fire, discover() should be tried, http_probe should fail
sleep 15
echo
echo "=== t+~20s after agent start (~15s after krakend container) ==="
echo "--- krakend listening test (expect connection refused): ---"
curl -s -o /dev/null -w "%{http_code}\n" --max-time 2 http://localhost:9090/metrics 2>&1 || echo "(curl failed, krakend not yet listening)"
echo
echo "--- agent log: discovery / krakend / discover / probe events (last 1m) ---"
docker logs --since 60s "$AGENT_NAME" 2>&1 | grep -iE "discover|krakend|probe|autodiscovery" | head -30
echo
echo "--- agent configcheck: krakend section? ---"
docker exec "$AGENT_NAME" agent configcheck 2>&1 | grep -B1 -A5 "krakend " | head -10 || echo "(no krakend section)"

# Phase 2: wait until krakend should be listening
NOW=$(date +%s)
WAIT=$((KRAKEND_LISTEN_EPOCH - NOW + 5))
if [ "$WAIT" -gt 0 ]; then
  echo
  echo "=== sleeping ${WAIT}s for krakend to start listening ==="
  sleep "$WAIT"
fi

echo
echo "=== t=$(date +%H:%M:%S) — krakend should now be listening ==="
echo "--- krakend listening test (expect 200): ---"
curl -s -o /dev/null -w "%{http_code}\n" --max-time 5 http://localhost:9090/metrics

echo
echo "=== now wait another 30s past the cache TTL and re-check ==="
sleep 30
echo
echo "--- agent log: any new discovery/krakend events? (last 30s) ---"
docker logs --since 30s "$AGENT_NAME" 2>&1 | grep -iE "discover|krakend|probe|autodiscovery" | head -20
echo
echo "--- agent configcheck: krakend section? ---"
docker exec "$AGENT_NAME" agent configcheck 2>&1 | grep -B1 -A8 "krakend " | head -15 || echo "(no krakend section)"
echo
echo "--- agent status: krakend? ---"
docker exec "$AGENT_NAME" agent status 2>&1 | sed -n '/krakend (/,/^$/p' | head -20 || echo "(no krakend section)"
echo
echo "=== full picture: discover-related log lines from full agent run ==="
docker logs "$AGENT_NAME" 2>&1 | grep -iE "discover|autodiscovery.*krakend" | head -40
