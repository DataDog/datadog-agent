#!/usr/bin/env bash
set -euo pipefail

fail() { echo "$*" >&2; exit 1; }

OUT="${1:?Usage: validate-recording.sh OUTPUT_DIR [FLUSH_SECONDS] [--backend vm|eks] [options]}"
shift
flush=""
if [[ $# -gt 0 && "${1:-}" != --* ]]; then
    flush="$1"
    shift
fi

backend=""
run_id=""
scenario_dir=""
episode_name=""
cluster_name=""
namespace=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --backend) backend="${2:?Missing value for --backend}"; shift 2 ;;
        --run-id) run_id="${2:-}"; shift 2 ;;
        --scenario-dir) scenario_dir="${2:-}"; shift 2 ;;
        --episode) episode_name="${2:-}"; shift 2 ;;
        --cluster-name) cluster_name="${2:-}"; shift 2 ;;
        --namespace) namespace="${2:-}"; shift 2 ;;
        *) fail "Unknown option: $1" ;;
    esac
done

[[ -d "$OUT" ]] || fail "Output directory does not exist: $OUT"
if [[ -n "$flush" ]]; then
    [[ "$flush" =~ ^[1-9][0-9]*$ ]] || fail "Flush interval must be positive seconds"
fi

script_dir="$(cd "$(dirname "$0")" && pwd)"
repo_root="$(cd "$script_dir/../../../../.." && pwd)"
recording_py="$repo_root/.agents/skills/anomalydetection-run-gensim-scenario-on-vm/scripts/recording.py"
[[ -f "$recording_py" ]] || fail "Recording validator not found: $recording_py"

if python3 -c 'import pyarrow' >/dev/null 2>&1; then
    py_cmd=(python3)
elif command -v uv >/dev/null 2>&1; then
    py_cmd=(uv run --no-project --with pyarrow python3)
else
    py_cmd=(python3)
fi

exec "${py_cmd[@]}" "$recording_py" validate "$OUT" "$flush" \
    --backend "$backend" \
    --run-id "$run_id" \
    --scenario-dir "$scenario_dir" \
    --episode "$episode_name" \
    --cluster-name "$cluster_name" \
    --namespace "$namespace"
