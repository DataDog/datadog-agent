#!/usr/bin/env bash
set -euo pipefail
. "$(dirname "$0")/vm.sh"

command="${1:?Usage: scenario.sh prepare|run --disposable-vm VM scenario-id [arguments]}"
[[ "${2:-}" == --disposable-vm ]] || fail "Requires --disposable-vm"
VM="${3:?Supply an SSH target}"
scenario="${4:?Supply a scenario id}"
target; name "$scenario"
shift 4

case "$command" in
    prepare)
        directory="${1:?Supply a local scenario directory}"
        [[ -f "$directory/play-episode.sh" && -d "$directory/chart" ]] || fail "Expected play-episode.sh and chart/"
        remote "$scenario" <<'EOF'
test ! -e "$HOME/gensim/$1"
mkdir -p "$HOME/gensim/$1"
EOF
        rsync -a --exclude results --exclude run-result.json -e "ssh -p $SSH_PORT" "$directory/" "$VM:~/gensim/$scenario/"
        remote "$scenario" <<'EOF'
cd "$HOME/gensim/$1"
rm -f -- chart/templates/datadog-agent.yaml
if [[ -f docker-compose.yaml || -f docker-compose.yml || -f compose.yaml || -f compose.yml ]]; then
    docker compose build
    images_text="$(docker compose config --images)"
    test -n "$images_text"
    mapfile -t images <<< "$images_text"
    docker image save -o "$HOME/gensim/scenario.tar" "${images[@]}"
    kind load image-archive "$HOME/gensim/scenario.tar" --name gadget-dev
    rm -- "$HOME/gensim/scenario.tar"
fi
EOF
        ;;
    run)
        episode="${1:?Supply an episode name}"; release="${2:?Supply a release name}"
        mode="${3:-full}"; dd_env="${4:-qbranch_gensim}"
        name "$episode"; name "$release"; name "$dd_env"
        [[ "$mode" == full || "$mode" == shortened ]] || fail "Mode must be full or shortened"
        remote "$scenario" "$episode" "$release" "$mode" "$dd_env" <<'EOF'
cd "$HOME/gensim/$1"
test ! -e "results/$2-1.json"
if [[ "$4" == shortened ]]; then
    sed -i -E 's/^WARMUP_DURATION=.*/WARMUP_DURATION=30/; s/^BASELINE_DURATION=.*/BASELINE_DURATION=60/; s/^DISRUPTION_DURATION=.*/DISRUPTION_DURATION=90/; s/^COOLDOWN_DURATION=.*/COOLDOWN_DURATION=30/' play-episode.sh
    touch .shortened
else
    test ! -e .shortened || { echo "Recopy the original scenario before a full run" >&2; exit 1; }
fi
set -a; . "$HOME/gensim/env"; set +a
export DD_ENV="$5" KUBE_NAMESPACE=default
helm --kube-context kind-gadget-dev --namespace default upgrade --install "$3" ./chart --set-string datadog.env="$DD_ENV"
kubectl --context kind-gadget-dev --namespace default wait --for=condition=Ready pod --all --timeout=300s
mkdir -p results
kubectl --context kind-gadget-dev --namespace default get pods -l app.kubernetes.io/component=agent -o json > "results/$2-agent-before.json"
nohup ./play-episode.sh run-episode "$2" > "results/$2.log" 2>&1 < /dev/null &
episode_pid=$!
if wait "$episode_pid"; then episode_status=0; else episode_status=$?; fi
kubectl --context kind-gadget-dev --namespace default get pods -l app.kubernetes.io/component=agent -o json > "results/$2-agent-after.json"
tail -20 "results/$2.log"
test "$episode_status" -eq 0
test -s "results/$2-1.json"
EOF
        ;;
    *) fail "Unknown command: $command" ;;
esac
