#!/usr/bin/env bash
set -euo pipefail
. "$(dirname "$0")/vm.sh"

VM="${1:?Usage: fetch.sh VM scenario-id episode output-dir flush-seconds}"
scenario="${2:?Supply a scenario id}"; episode="${3:?Supply an episode name}"
OUT="${4:?Supply a fresh output directory}"; flush="${5:?Supply the flush interval in seconds}"
target; name "$scenario"; name "$episode"
[[ "$OUT" == /* && "$OUT" != / ]] || fail "Output must be a fresh absolute directory"
[[ "$flush" =~ ^[1-9][0-9]*$ ]] || fail "Flush interval must be positive seconds"
mkdir -p -- "$(dirname "$OUT")"
mkdir -- "$OUT"
mkdir -- "$OUT/parquet"
scp -P "$SSH_PORT" "$VM:~/gensim/$scenario/results/$episode-1.json" "$OUT/episode.json"
snapshots="$(remote "$scenario" "$episode" <<'EOF'
if [[ -f "$HOME/gensim/$1/results/$2-agent-before.json" && -f "$HOME/gensim/$1/results/$2-agent-after.json" ]]; then
    printf present
else
    printf absent
fi
EOF
)"
if [[ "$snapshots" == present ]]; then
    scp -P "$SSH_PORT" "$VM:~/gensim/$scenario/results/$episode-agent-before.json" "$OUT/agent-before.json"
    scp -P "$SSH_PORT" "$VM:~/gensim/$scenario/results/$episode-agent-after.json" "$OUT/agent-after.json"
fi

wait_seconds="$(python3 "$SKILL_ROOT/scripts/recording.py" wait "$OUT/episode.json" "$flush")"
sleep "$wait_seconds"
remote <<'EOF' > "$OUT/agent-fetch.json"
kubectl --context kind-gadget-dev --namespace default get pods -l app.kubernetes.io/component=agent -o json
EOF
remote <<'EOF' > "$OUT/recording.tar"
pod="$(kubectl --context kind-gadget-dev --namespace default get pod -l app.kubernetes.io/component=agent -o name)"
[[ -n "$pod" && "$pod" != *$'\n'* ]]
kubectl --context kind-gadget-dev --namespace default exec "$pod" -c agent -- tar cf - -C /tmp/observer-metrics .
EOF

python3 "$SKILL_ROOT/scripts/recording.py" validate "$OUT" "$flush"
