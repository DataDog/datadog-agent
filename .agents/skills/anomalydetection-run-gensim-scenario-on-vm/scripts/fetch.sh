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

wait_seconds="$(python3 - "$OUT/episode.json" "$flush" <<'PY'
import datetime as dt
import json
import math
import sys

with open(sys.argv[1]) as source:
    episode = json.load(source)
end = dt.datetime.fromisoformat(episode["end_time"].replace("Z", "+00:00"))
if end.tzinfo is None or end > dt.datetime.now(dt.timezone.utc):
    raise SystemExit("episode end_time must be in the past and timezone-aware")
print(max(0, math.ceil(end.timestamp() + int(sys.argv[2]) - dt.datetime.now(dt.timezone.utc).timestamp())))
PY
)"
sleep "$wait_seconds"
remote <<'EOF' > "$OUT/agent-fetch.json"
kubectl --context kind-gadget-dev --namespace default get pods -l app.kubernetes.io/component=agent -o json
EOF
remote <<'EOF' > "$OUT/recording.tar"
pod="$(kubectl --context kind-gadget-dev --namespace default get pod -l app.kubernetes.io/component=agent -o name)"
[[ -n "$pod" && "$pod" != *$'\n'* ]]
kubectl --context kind-gadget-dev --namespace default exec "$pod" -c agent -- tar cf - -C /tmp/observer-metrics .
EOF

python3 - "$OUT" "$flush" <<'PY'
import datetime as dt
import json
from pathlib import Path, PurePosixPath
import shutil
import sys
import tarfile

import pyarrow as pa
import pyarrow.compute as pc
import pyarrow.parquet as pq

output = Path(sys.argv[1])
flush = int(sys.argv[2])

def identity(filename):
    pods = json.loads((output / filename).read_text())["items"]
    if len(pods) != 1:
        raise ValueError("expected one recorder Agent pod")
    pod = pods[0]
    containers = pod.get("status", {}).get("containerStatuses", [])
    if not containers or any(not item["ready"] or item["restartCount"] for item in containers):
        raise ValueError("Agent containers are unready or have restarted")
    return pod["metadata"]["uid"], {item["name"]: item["containerID"] for item in containers}

current_identity = identity("agent-fetch.json")
history_verified = (output / "agent-before.json").exists()
if history_verified and not identity("agent-before.json") == identity("agent-after.json") == current_identity:
    raise ValueError("Agent changed between the episode and fetching")

with tarfile.open(output / "recording.tar") as archive:
    members = archive.getmembers()
    names = set()
    for member in members:
        path = PurePosixPath(member.name)
        if member.isdir() and member.name in (".", "./"):
            continue
        if not member.isfile() or path.is_absolute() or len(path.parts) != 1 or path.name == ".." or path.name in names:
            raise ValueError(f"unsafe or duplicate archive entry: {member.name}")
        names.add(path.name)
    for member in members:
        if member.isfile():
            with archive.extractfile(member) as source, (output / "parquet" / PurePosixPath(member.name).name).open("xb") as destination:
                shutil.copyfileobj(source, destination)

episode = json.loads((output / "episode.json").read_text())
def timestamp(value):
    result = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    if result.tzinfo is None:
        raise ValueError("episode timestamps must include a timezone")
    return result.timestamp()

start, end = timestamp(episode["start_time"]), timestamp(episode["end_time"])
if start >= end:
    raise ValueError("episode window must have positive duration")
families = {family: {"files": 0, "rows": 0, "times": set()} for family in ("metrics", "logs")}
dropped = []
def integer_divisor(value):
    for divisor in (1, 1000, 1000000, 1000000000):
        if 1e9 < value / divisor < 4e9:
            return divisor
    raise ValueError(f"unexpected Time value {value}")

for filename in sorted((output / "parquet").glob("*.parquet")):
    family = next((key for key in families if filename.name.startswith(f"observer-{key}-")), None)
    if family is None:
        raise ValueError(f"unexpected parquet family: {filename.name}")
    table = pq.read_table(filename)
    column = table.column("Time")
    limits = pc.min_max(column).as_py() if len(column) else None
    if not limits or limits["min"] is None:
        dropped.append(filename)
        continue
    if pa.types.is_timestamp(column.type):
        divisor = {"s": 1, "ms": 1000, "us": 1000000, "ns": 1000000000}[column.type.unit]
    elif pa.types.is_integer(column.type):
        divisor = integer_divisor(limits["min"])
    else:
        raise ValueError(f"unsupported Time column: {column.type}")
    times = [value / divisor for value in pc.unique(pc.cast(column, pa.int64())).to_pylist() if value is not None]
    inside = [value for value in times if start <= value <= end]
    if max(times) < start or min(times) > end:
        dropped.append(filename)
        continue
    raw_time = pc.cast(column, pa.int64())
    selected = pc.and_(pc.greater_equal(raw_time, int(start * divisor)), pc.less_equal(raw_time, int(end * divisor)))
    families[family]["files"] += 1
    families[family]["rows"] += pc.sum(pc.cast(selected, pa.int64())).as_py()
    families[family]["times"].update(inside)

errors = []
warnings = [] if history_verified else ["no episode Agent snapshots; restart/replacement history is unknown"]
for family, stats in families.items():
    times = sorted(stats.pop("times"))
    if not times:
        errors.append(f"no nonempty {family} files overlap the episode")
        stats.update(observed_window=None, max_gap_seconds=None)
        continue
    gaps = [times[0] - start, end - times[-1]] + [later - earlier for earlier, later in zip(times, times[1:])]
    stats.update(observed_window=[times[0], times[-1]], max_gap_seconds=max(gaps))
    if gaps[0] > flush or gaps[1] > flush or any(gap > 2 * flush for gap in gaps[2:]):
        warnings.append(f"{family} has sparse timestamps or gaps; inspect coverage before claiming a complete recording")

report = {"usable": not errors, "window": [episode["start_time"], episode["end_time"]],
          "flush_seconds": flush, "families": families, "removed": len(dropped) if not errors else 0,
          "errors": errors, "coverage_warnings": warnings, "agent_history_verified": history_verified}
(output / "validation.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps(report, indent=2))
if errors:
    raise SystemExit("recording validation failed; keep partial output for diagnosis, do not replay")
for filename in dropped:
    filename.unlink()
(output / "recording.tar").unlink()
PY
