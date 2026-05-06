#!/usr/bin/env bash
set -euo pipefail

# observer-eval gs-flow generator
#
# gs-flow contract: read env vars, do work, write results to $OUTPUT_DIR, exit 0/1.
#
# Required env vars (set by gs-flow or job submission):
#   KUBECONFIG        - vcluster kubeconfig (set by gs-flow)
#   OUTPUT_DIR        - artifact output directory (set by gs-flow, typically /workspace)
#   DD_API_KEY        - Datadog API key
#   DD_APP_KEY        - Datadog app key
#   DD_SITE           - Datadog site (e.g. datadoghq.com)
#   AGENT_IMAGE       - full agent Docker image (e.g. docker.io/datadog/agent-dev:my-tag)
#   EPISODES          - comma-separated episode:scenario pairs
#
# Optional env vars:
#   GENSIM_MODE       - scrappy-collect (default), record-parquet, or live-anomaly-detection
#   KUBE_NAMESPACE    - namespace for workloads (default: default)
#   EPISODE_CHART_DIR - path to episode chart tarballs (mounted by gs-flow)
#   LOGS_ENABLED      - true/false (default: false)

OUTPUT_DIR="${OUTPUT_DIR:-/job/artifacts}"
KUBE_NAMESPACE="${KUBE_NAMESPACE:-default}"
GENSIM_MODE="${GENSIM_MODE:-scrappy-collect}"
LOGS_ENABLED="${LOGS_ENABLED:-false}"
GAR_REGISTRY="${GAR_REGISTRY:-us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images}"

# Derive mode flags
SCRAPPY_ENABLED="false"
DETECTORS_ENABLED="true"
if [ "$GENSIM_MODE" = "scrappy-collect" ]; then
  SCRAPPY_ENABLED="true"
  DETECTORS_ENABLED="false"
fi

# Parse image into repo + tag
AGENT_IMAGE_REPO="${AGENT_IMAGE%:*}"
AGENT_IMAGE_TAG="${AGENT_IMAGE##*:}"

# Export for envsubst in agent values template
export AGENT_IMAGE_REPO AGENT_IMAGE_TAG SCRAPPY_ENABLED DETECTORS_ENABLED LOGS_ENABLED GAR_REGISTRY

echo "observer-eval generator starting"
echo "  Mode:       $GENSIM_MODE"
# shellcheck disable=SC2153
echo "  Episodes:   $EPISODES"
echo "  Image:      $AGENT_IMAGE"
echo "  Output:     $OUTPUT_DIR"
echo "  Namespace:  $KUBE_NAMESPACE"

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

# Setup helm (for episode charts)
helm repo add datadog https://helm.datadoghq.com 2>/dev/null || true
helm repo update

# Wait for vcluster API to be ready
echo "Waiting for vcluster API..."
for _w in $(seq 1 30); do
  kubectl get ns "$KUBE_NAMESPACE" >/dev/null 2>&1 && break
  sleep 5
  echo "  vcluster not ready ($(_w)/30)..."
done

# Create DD secrets in the vcluster
kubectl create secret generic gensim-secrets \
  --from-literal=api-key="$DD_API_KEY" \
  --from-literal=app-key="$DD_APP_KEY" \
  -n "$KUBE_NAMESPACE" \
  --dry-run=client -o yaml | kubectl apply -f -

# Create GAR imagePullSecret so vcluster pods can pull from the private registry
GAR_HOST="${GAR_REGISTRY%%/*}"
if [ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ] && [ -f "${GOOGLE_APPLICATION_CREDENTIALS}" ]; then
  echo "Creating GAR imagePullSecret from service account key..."
  kubectl create secret docker-registry gar-pull-secret \
    --docker-server="https://$GAR_HOST" \
    --docker-username=_json_key \
    --docker-password="$(cat "$GOOGLE_APPLICATION_CREDENTIALS")" \
    -n "$KUBE_NAMESPACE" \
    --dry-run=client -o yaml | kubectl apply -f -
else
  # Try to get a short-lived access token from the metadata server
  echo "Creating GAR imagePullSecret from access token..."
  GAR_TOKEN=$(curl -sf -H "Metadata-Flavor: Google" \
    "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token" 2>/dev/null \
    | jq -r '.access_token' 2>/dev/null || true)
  if [ -n "$GAR_TOKEN" ] && [ "$GAR_TOKEN" != "null" ]; then
    kubectl create secret docker-registry gar-pull-secret \
      --docker-server="https://$GAR_HOST" \
      --docker-username=oauth2accesstoken \
      --docker-password="$GAR_TOKEN" \
      -n "$KUBE_NAMESPACE" \
      --dry-run=client -o yaml | kubectl apply -f -
  else
    echo "WARN: no GAR credentials available for imagePullSecret"
  fi
fi

# Patch default service account to use the pull secret automatically
kubectl patch serviceaccount default -n "$KUBE_NAMESPACE" \
  -p '{"imagePullSecrets": [{"name": "gar-pull-secret"}]}' 2>/dev/null || true

# Render agent values
envsubst < /templates/agent-values.yaml.tmpl > "$OUTPUT_DIR/agent-values.yaml"

# Post-renderer: swap agent image to our observer build, fix pull policy, inject observer env vars
cat > "$OUTPUT_DIR/fix-pull-policy.sh" <<FIXEOF
#!/bin/sh
sed -e 's/imagePullPolicy: Never/imagePullPolicy: Always/g' \
    -e 's|image: gcr.io/datadoghq/agent:7|image: $AGENT_IMAGE|g' \
    -e '/DD_DOGSTATSD_NON_LOCAL_TRAFFIC/{
a\\
            - name: DD_OBSERVER_RECORDING_ENABLED\\
              value: "true"\\
            - name: DD_OBSERVER_ANALYSIS_ENABLED\\
              value: "true"\\
            - name: DD_OBSERVER_HIGH_FREQUENCY_SYSTEM_CHECKS_ENABLED\\
              value: "true"\\
            - name: DD_OBSERVER_COMPONENTS_SCRAPPY_COLLECTOR_ENABLED\\
              value: "$SCRAPPY_ENABLED"\\
            - name: DD_OBSERVER_COMPONENTS_SCRAPPY_COLLECTOR_OUTPUT_PATH\\
              value: "/tmp/scrappy-collect.jsonl"
}'
FIXEOF
chmod +x "$OUTPUT_DIR/fix-pull-policy.sh"

# ── Cleanup stale resources from previous runs (vcluster may be reused) ──
echo "Cleaning up stale resources..."
for rel in $(helm ls -n "$KUBE_NAMESPACE" -a -q 2>/dev/null); do
  echo "  Uninstalling stale helm release: $rel"
  helm uninstall "$rel" -n "$KUBE_NAMESPACE" --wait 2>/dev/null || true
done
kubectl delete deployment,service,configmap -l app=datadog-agent -n "$KUBE_NAMESPACE" 2>/dev/null || true
kubectl delete pods --all -n "$KUBE_NAMESPACE" --force --grace-period=0 2>/dev/null || true
echo "Cleanup done."

# ── Main loop ────────────────────────────────────────────────────────────
IFS=',' read -ra EP_LIST <<< "$EPISODES"

for EP_SPEC in "${EP_LIST[@]}"; do
  EPISODE="${EP_SPEC%%:*}"
  SCENARIO="${EP_SPEC##*:}"
  EP_START=$(date +%s)

  echo "=== Episode: $EPISODE / $SCENARIO ==="

  # 1. Episode service images
  # Images are pre-built and pushed to GAR by build.sh. If DinD is available
  # (e.g. for iterating without a full rebuild), build them here as a fallback.
  EP_DIR="${EPISODE_CHART_DIR:-/episodes}/$EPISODE"
  if [ -f "$EP_DIR/docker-compose.yaml" ] && [ -n "${DOCKER_HOST:-}" ] && docker info >/dev/null 2>&1; then
    echo "DinD available — building episode service images..."

    # Authenticate with GAR
    if [ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]; then
      cat "$GOOGLE_APPLICATION_CREDENTIALS" | docker login -u _json_key --password-stdin "https://${GAR_REGISTRY%%/*}" 2>/dev/null || true
    fi

    # Build each service individually (docker compose plugin may not be available)
    if [ -d "$EP_DIR/services" ]; then
      COMPOSE_FILE="$EP_DIR/docker-compose.yaml"
      for svc_dir in "$EP_DIR/services"/*/; do
        [ ! -f "$svc_dir/Dockerfile" ] && continue
        svc_name="$(basename "$svc_dir")"
        # Extract image name for this service from docker-compose.yaml
        svc_image=$(awk "/$svc_name:/{found=1} found && /image:/{print \$2; exit}" "$COMPOSE_FILE")
        if [ -n "$svc_image" ]; then
          echo "  Building $svc_image from $svc_dir"
          docker build -t "$svc_image" "$svc_dir"
          GAR_IMG="$GAR_REGISTRY/$svc_image"
          docker tag "$svc_image" "$GAR_IMG"
          echo "  Pushing $GAR_IMG"
          docker push "$GAR_IMG" || echo "WARN: failed to push $GAR_IMG"
        fi
      done
    fi
  elif [ -f "$EP_DIR/docker-compose.yaml" ]; then
    echo "No DinD — expecting pre-built images in $GAR_REGISTRY"
  fi

  # 2. Install episode chart with agent.enabled=true. The post-renderer swaps
  # the chart's hardcoded agent image with our observer build, so the SAME agent
  # that receives DogStatsD/APM/logs also runs the observer + scrappy collector.
  EP_RELEASE=""
  CHART_TARBALL="$EP_DIR/chart.tar.gz"
  if [ -f "$CHART_TARBALL" ]; then
    mkdir -p "$OUTPUT_DIR/chart-$EPISODE"
    tar xzf "$CHART_TARBALL" -C "$OUTPUT_DIR/chart-$EPISODE"
    CHART_DIR=$(find "$OUTPUT_DIR/chart-$EPISODE" -maxdepth 1 -mindepth 1 -type d | head -1)
    [ -z "$CHART_DIR" ] && CHART_DIR="$OUTPUT_DIR/chart-$EPISODE"

    EP_RELEASE="gensim-$(echo "$EPISODE" | tr '_' '-' | tr '[:upper:]' '[:lower:]' | cut -c1-46)"

    echo "Installing episode chart from $CHART_DIR"
    echo "  Release: $EP_RELEASE"
    echo "  imageRegistry: $GAR_REGISTRY"

    # Dry-run first to see rendered manifests
    helm install "$EP_RELEASE" "$CHART_DIR" \
      --set agent.enabled=true \
      --set imageRegistry="${GAR_REGISTRY:-}" \
      --set namespace="$KUBE_NAMESPACE" \
      --set datadog.apiKey="REDACTED" \
      --set datadog.appKey="REDACTED" \
      --set datadog.site="$DD_SITE" \
      --set datadog.env="$EP_RELEASE" \
      -n "$KUBE_NAMESPACE" \
      --post-renderer "$OUTPUT_DIR/fix-pull-policy.sh" \
      --dry-run 2>&1 | head -100 || true
    echo "---"

    # No --wait: episode services may intentionally be degraded (503, crash-loops).
    # play-episode.sh handles its own readiness orchestration.
    if ! helm install "$EP_RELEASE" "$CHART_DIR" \
      --set agent.enabled=true \
      --set imageRegistry="${GAR_REGISTRY:-}" \
      --set namespace="$KUBE_NAMESPACE" \
      --set datadog.apiKey="$DD_API_KEY" \
      --set datadog.appKey="$DD_APP_KEY" \
      --set datadog.site="$DD_SITE" \
      --set datadog.env="$EP_RELEASE" \
      -n "$KUBE_NAMESPACE" \
      --post-renderer "$OUTPUT_DIR/fix-pull-policy.sh" \
      --timeout 5m 2>&1; then
      echo "--- Episode chart install FAILED ---"
      kubectl get pods -n "$KUBE_NAMESPACE" -o wide 2>/dev/null || true
    fi

    # Wait for at least one pod to be running (images pulled) before proceeding
    echo "Waiting for episode pods to start..."
    for _w in $(seq 1 30); do
      RUNNING=$(kubectl get pods -n "$KUBE_NAMESPACE" -o jsonpath='{.items[*].status.phase}' 2>/dev/null | tr ' ' '\n' | grep -c Running || true)
      [ "$RUNNING" -ge 2 ] && break
      sleep 10
    done
    kubectl get pods -n "$KUBE_NAMESPACE" -o wide || true
  fi

  # 4. Run play-episode.sh
  PLAY_SCRIPT="${EPISODE_CHART_DIR:-/episodes}/$EPISODE/play-episode.sh"
  SCENARIO_FILE="${EPISODE_CHART_DIR:-/episodes}/$EPISODE/episodes/$SCENARIO.yaml"

  if [ -f "$PLAY_SCRIPT" ]; then
    cp "$PLAY_SCRIPT" "$OUTPUT_DIR/play-episode.sh"
    chmod +x "$OUTPUT_DIR/play-episode.sh"
    mkdir -p "$OUTPUT_DIR/episodes"
    cp "$SCENARIO_FILE" "$OUTPUT_DIR/episodes/$SCENARIO.yaml"

    # Copy chart directory so play-episode.sh helm upgrade can find it
    if [ -d "$EP_DIR/chart" ]; then
      cp -r "$EP_DIR/chart" "$OUTPUT_DIR/chart" 2>/dev/null || true
    elif [ -n "${CHART_DIR:-}" ] && [ -d "${CHART_DIR:-}" ]; then
      cp -r "$CHART_DIR" "$OUTPUT_DIR/chart" 2>/dev/null || true
    fi

    # Inject shared helpers if available
    SHARED_ENV="${EPISODE_CHART_DIR:-/episodes}/_shared/env.sh"
    if [ -f "$SHARED_ENV" ]; then
      cp "$SHARED_ENV" "$OUTPUT_DIR/_shared_env.sh"
      sed -i '1s|^|source '"$OUTPUT_DIR"'/_shared_env.sh\n|' "$OUTPUT_DIR/play-episode.sh"
    fi

    # Disable set -e in play-episode.sh — episodes have expected failures
    # (duplicate monitors, degraded services, etc.) that shouldn't abort the run
    sed -i 's/^set -euo pipefail/set -uo pipefail/' "$OUTPUT_DIR/play-episode.sh"

    export DD_ENV="$EP_RELEASE"
    export DD_API_KEY DD_APP_KEY KUBE_NAMESPACE DD_SITE

    # Start background artifact collector — copies scrappy/parquet from agent pod
    # every 60s while the episode runs. Protects against vcluster teardown race.
    # Find the agent pod (chart's agent, now running our observer binary)
    AGENT_POD_FOR_BG=$(kubectl get pod -n "$KUBE_NAMESPACE" -l app=datadog-agent -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    if [ -n "$AGENT_POD_FOR_BG" ]; then
      (
        while true; do
          sleep 60
          # Copy scrappy JSONL to artifact root (gs-flow only serves flat files)
          kubectl cp "$KUBE_NAMESPACE/$AGENT_POD_FOR_BG:/tmp/scrappy-collect.jsonl" \
            "$OUTPUT_DIR/scrappy-collect.jsonl" -c agent 2>/dev/null || true
          # Copy parquet — tar locally since gs-flow only serves flat files
          rm -rf "$OUTPUT_DIR/_pq_staging" 2>/dev/null
          kubectl cp "$KUBE_NAMESPACE/$AGENT_POD_FOR_BG:/tmp/observer-parquet" \
            "$OUTPUT_DIR/_pq_staging" -c agent 2>/dev/null && \
          tar czf "$OUTPUT_DIR/observer-parquet.tar.gz" -C "$OUTPUT_DIR/_pq_staging" . 2>/dev/null || true
          rm -rf "$OUTPUT_DIR/_pq_staging" 2>/dev/null
          LINES=$(wc -l < "$OUTPUT_DIR/scrappy-collect.jsonl" 2>/dev/null || echo 0)
          PQCOUNT=$(tar tzf "$OUTPUT_DIR/observer-parquet.tar.gz" 2>/dev/null | grep -c '.parquet$' || echo 0)
          PQSIZE=$(du -sh "$OUTPUT_DIR/observer-parquet.tar.gz" 2>/dev/null | awk '{print $1}' || echo "0")
          echo "  [bg-collector] scrappy: $LINES lines, parquet: $PQCOUNT files ($PQSIZE)"
        done
      ) &
      BG_COLLECTOR_PID=$!
      echo "Background artifact collector started (pid=$BG_COLLECTOR_PID, pod=$AGENT_POD_FOR_BG)"
    fi

    EP_OUTCOME="success"
    cd "$OUTPUT_DIR"
    bash "$OUTPUT_DIR/play-episode.sh" run-episode "$SCENARIO" || EP_OUTCOME="failure"
    cd /

    # Stop background collector
    [ -n "${BG_COLLECTOR_PID:-}" ] && kill "$BG_COLLECTOR_PID" 2>/dev/null || true

    # Copy episode timeline to artifact root (gs-flow only serves flat files)
    TIMELINE="$OUTPUT_DIR/results/${SCENARIO}-1.json"
    [ -f "$TIMELINE" ] && cp "$TIMELINE" "$OUTPUT_DIR/episode-timeline.json"

    # Capture agent logs (includes scrappy debug output on stderr)
    if [ -n "${AGENT_POD_FOR_BG:-}" ]; then
      echo "--- Agent logs (last 50 lines) ---"
      kubectl logs "$AGENT_POD_FOR_BG" -n "$KUBE_NAMESPACE" -c agent --tail=50 2>&1 || true
    fi
  fi

  # 5. Collect results
  echo "--- Collecting artifacts from agent pod ---"
  echo "Looking for agent pod..."
  kubectl get pods -n "$KUBE_NAMESPACE" -o wide || true

  # Try multiple label selectors (helm chart version differences)
  AGENT_POD=""
  for selector in "app=datadog-agent" "app.kubernetes.io/name=datadog-agent-agent"; do
    AGENT_POD=$(kubectl get pod -n "$KUBE_NAMESPACE" -l "$selector" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    if [ -n "$AGENT_POD" ]; then
      echo "Found agent pod: $AGENT_POD (selector: $selector)"
      break
    fi
  done

  # Fallback: find agent DaemonSet pod by name (exclude cluster-agent, operator)
  if [ -z "$AGENT_POD" ]; then
    AGENT_POD=$(kubectl get pods -n "$KUBE_NAMESPACE" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null \
      | tr ' ' '\n' \
      | grep -i 'agent' \
      | grep -v -E 'cluster-agent|operator' \
      | head -1 || true)
    [ -n "$AGENT_POD" ] && echo "Found agent pod by name match: $AGENT_POD"
  fi

  mkdir -p "$OUTPUT_DIR/results/$EPISODE/parquet"
  mkdir -p "$OUTPUT_DIR/results/$EPISODE/scrappy"

  if [ -n "$AGENT_POD" ]; then
    echo "Collecting from pod $AGENT_POD..."

    # Check what's actually in /tmp on the agent
    kubectl exec "$AGENT_POD" -n "$KUBE_NAMESPACE" -c agent -- ls -la /tmp/ 2>&1 || echo "WARN: could not list /tmp"
    kubectl exec "$AGENT_POD" -n "$KUBE_NAMESPACE" -c agent -- ls -la /tmp/observer-parquet/ 2>&1 || echo "WARN: /tmp/observer-parquet not found"
    kubectl exec "$AGENT_POD" -n "$KUBE_NAMESPACE" -c agent -- ls -la /tmp/scrappy-collect.jsonl 2>&1 || echo "WARN: /tmp/scrappy-collect.jsonl not found"

    # Parquet
    if kubectl cp "$KUBE_NAMESPACE/$AGENT_POD:/tmp/observer-parquet" "$OUTPUT_DIR/results/$EPISODE/parquet/" -c agent; then
      PARQUET_COUNT=$(find "$OUTPUT_DIR/results/$EPISODE/parquet" -type f -name '*.parquet' | wc -l)
      echo "Parquet collected: $PARQUET_COUNT files"
    else
      echo "ERROR: parquet collection failed" >&2
    fi

    # Scrappy JSONL
    if kubectl cp "$KUBE_NAMESPACE/$AGENT_POD:/tmp/scrappy-collect.jsonl" "$OUTPUT_DIR/results/$EPISODE/scrappy/scrappy-collect.jsonl" -c agent; then
      SCRAPPY_LINES=$(wc -l < "$OUTPUT_DIR/results/$EPISODE/scrappy/scrappy-collect.jsonl")
      echo "Scrappy JSONL collected: $SCRAPPY_LINES lines"
    else
      echo "ERROR: scrappy JSONL collection failed" >&2
    fi
  else
    echo "ERROR: no agent pod found! Available pods:" >&2
    kubectl get pods -n "$KUBE_NAMESPACE" --show-labels >&2 || true
  fi

  EP_END=$(date +%s)
  EP_DURATION=$((EP_END - EP_START))

  # Write episode metadata
  cat > "$OUTPUT_DIR/results/$EPISODE/meta.json" <<EOF
{
  "episode": "$EPISODE",
  "scenario": "$SCENARIO",
  "outcome": "$EP_OUTCOME",
  "duration_seconds": $EP_DURATION,
  "agent_image": "$AGENT_IMAGE",
  "mode": "$GENSIM_MODE",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

  # 6. Teardown episode + agent for next iteration
  echo "Tearing down..."
  [ -n "$EP_RELEASE" ] && helm uninstall "$EP_RELEASE" -n "$KUBE_NAMESPACE" --wait 2>/dev/null || true
  kubectl delete -f "$OUTPUT_DIR/agent-deployment.yaml" --wait 2>/dev/null || true
  kubectl wait --for=delete pod -l app=datadog-agent -n "$KUBE_NAMESPACE" --timeout=120s 2>/dev/null || true

  echo "=== Episode $EPISODE / $SCENARIO complete (${EP_DURATION}s, $EP_OUTCOME) ==="
done

echo "All episodes complete. Results in $OUTPUT_DIR/results/"
