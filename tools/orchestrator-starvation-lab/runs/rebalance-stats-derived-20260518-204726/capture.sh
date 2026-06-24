#!/usr/bin/env bash
set -euo pipefail
RUN=${1:?run dir}
DCA=$(kubectl -n datadog get pods -l app.kubernetes.io/component=cluster-agent -o jsonpath='{.items[0].metadata.name}')
mapfile -t RUNNERS < <(kubectl -n datadog get pods -l app.kubernetes.io/component=cluster-checks-runner --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
echo "DCA=$DCA" > "$RUN/capture-info.txt"
printf 'RUNNER=%s\n' "${RUNNERS[@]}" >> "$RUN/capture-info.txt"
(kubectl -n datadog logs -f "$DCA" > "$RUN/dca-live.log" 2>&1) & echo $! > "$RUN/dca-tail.pid"
for p in "${RUNNERS[@]}"; do (kubectl -n datadog logs -f "$p" > "$RUN/runner-$p.log" 2>&1) & echo $! >> "$RUN/runner-tail.pids"; done
end=$((SECONDS+600))
while (( SECONDS < end )); do
  ts=$(date -u +%FT%TZ)
  echo "=== $ts ===" >> "$RUN/status-orch.log"
  for p in "${RUNNERS[@]}"; do
    echo "--- $p ---" >> "$RUN/status-orch.log"
    kubectl -n datadog exec "$p" -- agent status 2>/dev/null | awk '/orchestrator \(/,/^$/' >> "$RUN/status-orch.log" || true
  done
  sleep 15
done
kill $(cat "$RUN/dca-tail.pid" "$RUN/runner-tail.pids") 2>/dev/null || true
