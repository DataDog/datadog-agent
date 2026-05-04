#!/usr/bin/env bash
set -uo pipefail

# Batch collect scrappy training data across multiple gensim episodes.
#
# Usage:
#   ./collect-batch.sh --episodes "008_Slack:haproxy-state-sync-failure,059_Fortnite:memcached-saturation"
#   ./collect-batch.sh --manifest episodes.txt   # one episode:scenario per line
#   ./collect-batch.sh --count 10                # first 10 Python-service episodes
#
# Requires:
#   - Local gs-flow running at localhost:8080
#   - GENSIM_REPO_PATH set
#   - DD_API_KEY, DD_APP_KEY set
#   - Agent image already pushed (see README.md)

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DATA_DIR="${SCRAPPY_DATA_DIR:-$HOME/dd/scrappy/data}"
GS_FLOW_URL="${GS_FLOW_URL:-http://localhost:8080}"
POLL_INTERVAL=30
MAX_CONCURRENT="${MAX_CONCURRENT:-1}"

# Read GAR registry from gs-flow config
SANDBOX_GAR=$(curl -s "$GS_FLOW_URL/api/v1/config/cluster" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('gar_registry',''))" 2>/dev/null)
if [ -z "$SANDBOX_GAR" ]; then
  echo "ERROR: Cannot read GAR registry from gs-flow at $GS_FLOW_URL"
  exit 1
fi

AGENT_IMAGE="${AGENT_IMAGE:-$SANDBOX_GAR/agent-dev:scrappy-collect-amd64}"

# Parse args
EPISODES=""
MANIFEST=""
COUNT=0

while [[ $# -gt 0 ]]; do
  case $1 in
    --episodes) EPISODES="$2"; shift 2;;
    --manifest) MANIFEST="$2"; shift 2;;
    --count) COUNT="$2"; shift 2;;
    --data-dir) DATA_DIR="$2"; shift 2;;
    --concurrent) MAX_CONCURRENT="$2"; shift 2;;
    *) echo "Unknown arg: $1"; exit 1;;
  esac
done

# Build episode list
EP_LIST=()
if [ -n "$EPISODES" ]; then
  IFS=',' read -ra EP_LIST <<< "$EPISODES"
elif [ -n "$MANIFEST" ]; then
  while IFS= read -r line; do
    line=$(echo "$line" | sed 's/#.*//' | xargs)
    [ -n "$line" ] && EP_LIST+=("$line")
  done < "$MANIFEST"
elif [ "$COUNT" -gt 0 ]; then
  echo "Discovering first $COUNT Python-service episodes..."
  while IFS= read -r ep_dir; do
    ep_name=$(basename "$ep_dir")
    [ "$ep_name" = "_shared" ] && continue
    [ -d "$ep_dir/chart" ] && [ -d "$ep_dir/episodes" ] || continue

    # Check all services are Python
    all_python=true
    for df in "$ep_dir"/services/*/Dockerfile; do
      [ -f "$df" ] || continue
      grep -qi "python\|flask\|django" "$df" || { all_python=false; break; }
    done
    $all_python || continue

    # Pick first scenario
    scenario=$(find "$ep_dir/episodes" -maxdepth 1 -name "*.yaml" 2>/dev/null | head -1 | xargs -I{} basename {} .yaml 2>/dev/null)
    [ -n "$scenario" ] && EP_LIST+=("$ep_name:$scenario")
    [ "${#EP_LIST[@]}" -ge "$COUNT" ] && break
  done < <(find "$GENSIM_REPO_PATH" -maxdepth 3 -name "play-episode.sh" -exec dirname {} \; 2>/dev/null | sort)
fi

if [ "${#EP_LIST[@]}" -eq 0 ]; then
  echo "No episodes specified. Use --episodes, --manifest, or --count."
  exit 1
fi

echo "=== Scrappy Batch Collection ==="
echo "  Episodes: ${#EP_LIST[@]}"
echo "  Data dir: $DATA_DIR"
echo "  GAR:      $SANDBOX_GAR"
echo "  Agent:    $AGENT_IMAGE"
echo ""

mkdir -p "$DATA_DIR"

# Submit and track jobs
COMPLETED=0
FAILED=0

for ep_spec in "${EP_LIST[@]}"; do
  EPISODE="${ep_spec%%:*}"
  SCENARIO="${ep_spec##*:}"
  SLUG=$(echo "${EPISODE}_${SCENARIO}" | tr '[:upper:]' '[:lower:]' | tr ' ' '-')

  # Skip if already collected
  if [ -f "$DATA_DIR/$SLUG/scrappy/scrappy-collect.jsonl" ]; then
    LINES=$(wc -l < "$DATA_DIR/$SLUG/scrappy/scrappy-collect.jsonl" 2>/dev/null)
    if [ "$LINES" -gt 10 ]; then
      echo "SKIP $EPISODE:$SCENARIO (already collected, $LINES lines)"
      COMPLETED=$((COMPLETED + 1))
      continue
    fi
  fi

  # Build generator image with this episode
  echo "BUILD $EPISODE:$SCENARIO"
  REGISTRY="$SANDBOX_GAR" "$SCRIPT_DIR/build.sh" --push --episodes "$ep_spec" >/dev/null 2>&1
  if [ $? -ne 0 ]; then
    echo "  FAIL: generator build failed"
    FAILED=$((FAILED + 1))
    continue
  fi

  # Submit job
  RESP=$(curl -s -X POST "$GS_FLOW_URL/api/v1/jobs" \
    -H "Content-Type: application/json" \
    -d "{
      \"backend\": \"observer-eval\",
      \"generator_image\": \"$SANDBOX_GAR/observer-eval:latest\",
      \"secrets\": {
        \"AGENT_IMAGE\": \"$AGENT_IMAGE\",
        \"EPISODES\": \"$ep_spec\",
        \"GENSIM_MODE\": \"scrappy-collect\",
        \"LOGS_ENABLED\": \"false\",
        \"DD_API_KEY\": \"$DD_API_KEY\",
        \"DD_APP_KEY\": \"$DD_APP_KEY\",
        \"DD_SITE\": \"${DD_SITE:-datadoghq.com}\",
        \"GAR_REGISTRY\": \"$SANDBOX_GAR\"
      }
    }")

  JOB_ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('job_id',''))" 2>/dev/null)
  if [ -z "$JOB_ID" ]; then
    echo "  FAIL: submit failed: $RESP"
    FAILED=$((FAILED + 1))
    continue
  fi

  echo "  SUBMIT $JOB_ID"
  # Wait for completion (sequential for now)
  while true; do
    sleep "$POLL_INTERVAL"
    STATUS=$(curl -s "$GS_FLOW_URL/api/v1/jobs/$JOB_ID" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','?'))" 2>/dev/null)
    case "$STATUS" in
      completed)
        # Harvest
        SRC="/tmp/gs-flow-artifacts/jobs/$JOB_ID/artifacts"
        DST="$DATA_DIR/$SLUG"
        mkdir -p "$DST/scrappy" "$DST/parquet" "$DST/meta"

        cp "$SRC/scrappy-collect.jsonl" "$DST/scrappy/scrappy-collect.jsonl" 2>/dev/null
        cp "$SRC/results/"*/parquet/*.parquet "$DST/parquet/" 2>/dev/null
        cp "$SRC/results/"*/meta.json "$DST/meta/meta.json" 2>/dev/null

        LINES=$(wc -l < "$DST/scrappy/scrappy-collect.jsonl" 2>/dev/null || echo 0)
        PQ=$(find "$DST/parquet" -name "*.parquet" 2>/dev/null | wc -l)
        echo "  DONE $EPISODE:$SCENARIO -> $LINES lines, $PQ parquet"
        COMPLETED=$((COMPLETED + 1))
        break
        ;;
      failed)
        ERR=$(curl -s "$GS_FLOW_URL/api/v1/jobs/$JOB_ID" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('error_message','unknown')[:100])" 2>/dev/null)
        echo "  FAIL $EPISODE:$SCENARIO: $ERR"
        FAILED=$((FAILED + 1))
        break
        ;;
    esac
  done
done

echo ""
echo "=== Batch Complete ==="
echo "  Completed: $COMPLETED"
echo "  Failed:    $FAILED"
echo "  Data dir:  $DATA_DIR"
