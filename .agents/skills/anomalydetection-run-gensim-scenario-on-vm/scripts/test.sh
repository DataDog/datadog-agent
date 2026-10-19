#!/usr/bin/env bash
set -euo pipefail

scripts="$(cd "$(dirname "$0")" && pwd)"
workspace="$(mktemp -d)"
trap 'rm -rf -- "$workspace"' EXIT
mkdir "$workspace/bin" "$workspace/fixture"
export FIXTURE="$workspace/fixture" CALLS="$workspace/calls"
export PATH="$workspace/bin:$PATH"

cat > "$workspace/bin/ssh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
script="$(cat)"
printf '%s\n' "$script" >> "$CALLS"
if [[ "$script" == *"tar cf"* ]]; then
    [[ "${FAIL_ARCHIVE:-0}" == 0 ]] || exit 1
    cat "$FIXTURE/recording.tar"
elif [[ "$script" == *"-o json"* ]]; then
    cat "$FIXTURE/agent-after.json"
elif [[ "$script" == *"printf present"* ]]; then
    if [[ "${NO_SNAPSHOTS:-0}" == 1 ]]; then printf absent; else printf present; fi
fi
SH
cat > "$workspace/bin/scp" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
destination="${@: -1}"
source="${@: -2:1}"
[[ "${FAIL_COPY:-0}" == 0 ]] || exit 1
if [[ "$source" == *:* ]]; then
    cp -- "$FIXTURE/$(basename "$destination")" "$destination"
else
    cp -- "$source" "$FIXTURE/upload-$(basename "$destination")"
fi
SH
cat > "$workspace/bin/dda" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$CALLS"
SH
chmod +x "$workspace/bin/"*

fixture() {
    python3 - "$FIXTURE" "$1" <<'PY'
import datetime as dt
import io
import json
from pathlib import Path
import sys
import tarfile

import pyarrow as pa
import pyarrow.parquet as pq

root, mode = Path(sys.argv[1]), sys.argv[2]
for filename in root.glob("*.parquet"):
    filename.unlink()
start = int(dt.datetime(2026, 1, 1, tzinfo=dt.timezone.utc).timestamp())
episode = {"start_time": "2026-01-01T00:00:00Z", "end_time": "2026-01-01T00:02:00Z"}
(root / "episode.json").write_text(json.dumps(episode))
pod = {"items": [{"metadata": {"uid": "agent"}, "status": {"containerStatuses": [
    {"name": "agent", "ready": True, "restartCount": 0, "containerID": "container"}
]}}]}
(root / "agent-before.json").write_text(json.dumps(pod))
if mode == "restart":
    pod["items"][0]["status"]["containerStatuses"][0]["restartCount"] = 1
if mode == "replacement":
    pod["items"][0]["metadata"]["uid"] = "replacement"
(root / "agent-after.json").write_text(json.dumps(pod))
for family in ("metrics", "logs"):
    values = [(start + offset) * 1000 for offset in (0, 30, 60, 90, 120)]
    if mode == "gap":
        values = [(start + 90) * 1000, (start + 120) * 1000]
    if mode == "outside":
        values = [(start - 300) * 1000]
    if mode == "empty":
        values = []
    if mode in ("seconds", "microseconds", "nanoseconds"):
        divisor = {"seconds": 1, "microseconds": 1000000, "nanoseconds": 1000000000}[mode]
        values = [(start + offset) * divisor for offset in (0, 30, 60, 90, 120)]
    column = pa.array(values, type=pa.int64())
    if mode == "timestamp":
        column = column.cast(pa.timestamp("ms", tz="UTC"))
    pq.write_table(pa.table({"Time": column, "Value": ["row"] * len(values)}), root / f"observer-{family}-inside.parquet")
    pq.write_table(pa.table({"Time": [(start - 300) * 1000]}), root / f"observer-{family}-outside.parquet")
    if mode == "straddle":
        pq.write_table(pa.table({"Time": [(start - 5) * 1000, (start + 125) * 1000]}),
                       root / f"observer-{family}-boundary.parquet")
if mode == "corrupt":
    (root / "observer-logs-inside.parquet").write_bytes(b"PAR1broken")
with tarfile.open(root / "recording.tar", "w") as archive:
    for filename in sorted(root.glob("*.parquet")):
        if mode == "missing-logs" and filename.name.startswith("observer-logs-"):
            continue
        archive.add(filename, arcname=f"./{filename.name}")
    extra = tarfile.TarInfo("./advances.jsonl")
    extra.size = 3
    archive.addfile(extra, io.BytesIO(b"{}\n"))
    if mode == "unsafe":
        extra = tarfile.TarInfo("../escape")
        extra.size = 1
        archive.addfile(extra, io.BytesIO(b"x"))
    if mode == "symlink":
        extra = tarfile.TarInfo("link")
        extra.type = tarfile.SYMTYPE
        extra.linkname = "/tmp"
        archive.addfile(extra)
PY
}

fetch() {
    bash "$scripts/fetch.sh" user@vm scenario episode "$1" 60
}

bash "$scripts/vm.sh" create-vm --disposable-vm skill-test t3.2xlarge
[[ "$(cat "$CALLS")" == "inv aws.create-vm --stack-name=skill-test --instance-type=t3.2xlarge --no-install-agent" ]]
if bash "$scripts/vm.sh" create-vm skill-test > /dev/null 2>&1; then
    echo "creation succeeded without authorization" >&2; exit 1
fi

printf 'DD_API_KEY=dummy-key\nDD_APP_KEY=dummy-app\n' > "$workspace/env"
bash "$scripts/vm.sh" create-cluster --disposable-vm user@vm
bash "$scripts/vm.sh" deploy-recorder --disposable-vm user@vm "$workspace/env" test-tag skill-test 1m 90m
python3 - "$FIXTURE/upload-recorder-values.yaml" <<'PY'
import sys
import yaml

with open(sys.argv[1]) as source:
    values = yaml.safe_load(source)
assert values["datadog"]["clusterName"] == "skill-test"
assert values["agents"]["image"]["tag"] == "test-tag"
settings = {item["name"]: item["value"] for item in values["datadog"]["env"]}
assert settings["DD_ANOMALY_DETECTION_RECORDING_FLUSH_INTERVAL"] == "1m"
assert settings["DD_ANOMALY_DETECTION_RECORDING_ONLY"] == "true"
PY
if grep -q dummy-key "$CALLS"; then echo "credential leaked into SSH script" >&2; exit 1; fi
if bash "$scripts/vm.sh" deploy-recorder --disposable-vm user@vm "$workspace/env" test-tag skill-test 2m 1m > /dev/null 2>&1; then
    echo "invalid retention accepted" >&2; exit 1
fi

for mode in valid timestamp seconds microseconds nanoseconds straddle gap; do
    fixture "$mode"
    output="$workspace/output with spaces-$mode"
    fetch "$output" > /dev/null
    python3 - "$output" "$mode" "$FIXTURE" <<'PY'
import datetime as dt
import json
from pathlib import Path
import sys

import pyarrow.compute as pc
import pyarrow.parquet as pq

output = Path(sys.argv[1])
report = json.loads((output / "validation.json").read_text())
assert report["usable"] and report["removed"] == 2
files, rows = (2, 5) if sys.argv[2] == "straddle" else (1, 2) if sys.argv[2] == "gap" else (1, 5)
assert all(stats["files"] == files and stats["rows"] == rows for stats in report["families"].values())
assert len(list((output / "parquet").glob("*.parquet"))) == 2 * files
assert bool(report["coverage_warnings"]) == (sys.argv[2] == "gap")
assert (output / "parquet/advances.jsonl").read_text() == "{}\n"
if sys.argv[2] != "timestamp":
    def seconds(value):
        for divisor in (1, 1e3, 1e6, 1e9):
            if 1e9 < value / divisor < 4e9:
                return value / divisor
        raise ValueError(f"unexpected Time value {value}")
    episode = json.loads((output / "episode.json").read_text())
    start, end = (dt.datetime.strptime(episode[key], "%Y-%m-%dT%H:%M:%SZ").replace(
        tzinfo=dt.timezone.utc).timestamp() for key in ("start_time", "end_time"))
    expected = set()
    for filename in Path(sys.argv[3]).glob("*.parquet"):
        column = pq.read_table(filename, columns=["Time"]).column(0)
        limits = pc.min_max(column).as_py() if len(column) else None
        if limits and seconds(limits["max"]) >= start and seconds(limits["min"]) <= end:
            expected.add(filename.name)
    assert {filename.name for filename in (output / "parquet").glob("*.parquet")} == expected
PY
    printf '%s' precious > "$output/sentinel"
    if fetch "$output" > /dev/null 2>&1; then echo "overwrote output" >&2; exit 1; fi
    [[ "$(cat "$output/sentinel")" == precious ]]
done

for mode in corrupt missing-logs outside empty unsafe symlink restart replacement; do
    fixture "$mode"
    if fetch "$workspace/$mode" > "$workspace/$mode.log" 2>&1; then
        echo "unexpected success: $mode" >&2; exit 1
    fi
done
fixture valid
NO_SNAPSHOTS=1 fetch "$workspace/legacy-recording" > /dev/null
python3 - "$workspace/legacy-recording/validation.json" <<'PY'
import json
import sys

with open(sys.argv[1]) as source:
    report = json.load(source)
assert report["usable"] and not report["agent_history_verified"]
assert report["coverage_warnings"]
PY
if FAIL_ARCHIVE=1 fetch "$workspace/ssh-failure" > /dev/null 2>&1; then echo "ignored SSH failure" >&2; exit 1; fi
if FAIL_COPY=1 fetch "$workspace/scp-failure" > /dev/null 2>&1; then echo "ignored SCP failure" >&2; exit 1; fi
[[ ! -e "$workspace/escape" ]]

# ── EKS orchestrator and multi-pod validation tests ──────────────────────────
repo_root="$(cd "$scripts/../../../.." && pwd)"
orchestrator_script="$repo_root/test/e2e-framework/scenarios/aws/gensim-eks/orchestrator.sh.tmpl"
validator_script="$repo_root/test/e2e-framework/scenarios/aws/gensim-eks/validate-recording.sh"

eks_root="$workspace/eks"
mkdir -p "$eks_root/bin" "$eks_root/s3" "$eks_root/k8s" "$eks_root/pods" "$eks_root/episodes/authcore-pgbouncer" "$eks_root/config"

python3 - "$eks_root" <<'PY'
import datetime as dt
import io
import json
from pathlib import Path
import sys
import tarfile

import pyarrow as pa
import pyarrow.parquet as pq

root = Path(sys.argv[1])
ep_mount = root / "episodes" / "authcore-pgbouncer"
chart_src = root / "chart-src" / "chart"
chart_src.mkdir(parents=True)
(chart_src / "Chart.yaml").write_text("apiVersion: v2\nname: test\nversion: 0.1.0\n")
with tarfile.open(ep_mount / "chart.tar.gz", "w:gz") as tar:
    tar.add(chart_src, arcname="chart")

(ep_mount / "play-episode.sh").write_text(
    '#!/usr/bin/env bash\n'
    'set -euo pipefail\n'
    'scen="$2"\n'
    'mkdir -p results\n'
    'printf \'{"episode":"authcore-pgbouncer","scenario":"%s","cycle":1,"start_time":"2026-01-01T00:00:00Z","end_time":"2026-01-01T00:02:00Z","success":false}\\n\' "$scen" > "results/${scen}-1.json"\n'
)
(ep_mount / "pool-saturation.yaml").write_text("name: pool-saturation\n")
(ep_mount / "conn-spike.yaml").write_text("name: conn-spike\n")
(root / "config" / "agent-values.yaml").write_text("fullnameOverride: datadog-agent\n")

start = int(dt.datetime(2026, 1, 1, tzinfo=dt.timezone.utc).timestamp())
for idx, pod_name in enumerate(("datadog-agent-node1", "datadog-agent-node2"), start=1):
    pod_dir = root / "pods" / pod_name
    pod_dir.mkdir(parents=True)
    for family in ("metrics", "logs"):
        inside_vals = [(start + offset + idx) * 1000 for offset in (0, 30, 60, 90)]
        outside_vals = [(start - 300) * 1000]
        # Both pods produce identically named files!
        pq.write_table(
            pa.table({"Time": pa.array(inside_vals, type=pa.int64()), "Pod": [pod_name] * len(inside_vals)}),
            pod_dir / f"observer-{family}-20260101-000100Z.parquet",
        )
        pq.write_table(
            pa.table({"Time": pa.array(outside_vals, type=pa.int64()), "Pod": [pod_name]}),
            pod_dir / f"observer-{family}-20251231-235500Z.parquet",
        )
    (pod_dir / "advances.jsonl").write_text(json.dumps({"pod": pod_name}) + "\n")

pods_healthy = {
    "items": [
        {
            "metadata": {"name": "datadog-agent-node1", "uid": "uid-1"},
            "spec": {"nodeName": "ip-10-0-1-10.ec2.internal", "containers": [{"name": "agent"}]},
            "status": {
                "containerStatuses": [
                    {
                        "name": "agent",
                        "ready": True,
                        "restartCount": 0,
                        "containerID": "containerd://agent-1",
                        "image": "docker.io/datadog/agent-dev:my-branch-recorder-full",
                        "imageID": "docker.io/datadog/agent-dev@sha256:1111",
                    }
                ]
            },
        },
        {
            "metadata": {"name": "datadog-agent-node2", "uid": "uid-2"},
            "spec": {"nodeName": "ip-10-0-1-11.ec2.internal", "containers": [{"name": "agent"}]},
            "status": {
                "containerStatuses": [
                    {
                        "name": "agent",
                        "ready": True,
                        "restartCount": 0,
                        "containerID": "containerd://agent-2",
                        "image": "docker.io/datadog/agent-dev:my-branch-recorder-full",
                        "imageID": "docker.io/datadog/agent-dev@sha256:1111",
                    }
                ]
            },
        },
    ]
}
(root / "k8s" / "pods-healthy.json").write_text(json.dumps(pods_healthy))

pods_restarted = json.loads(json.dumps(pods_healthy))
pods_restarted["items"][1]["status"]["containerStatuses"][0]["restartCount"] = 1
(root / "k8s" / "pods-restarted.json").write_text(json.dumps(pods_restarted))
PY

cat > "$eks_root/bin/helm" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'helm %s\n' "$*" >> "$EKS_CALLS"
if [[ "${1:-}" == "ls" ]]; then
    exit 0
fi
SH

cat > "$eks_root/bin/kubectl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'kubectl %s\n' "$*" >> "$EKS_CALLS"
args="$*"
if [[ "$args" == *"create configmap gensim-run-status"* ]]; then
    for arg in "$@"; do
        if [[ "$arg" == --from-literal=status=* ]]; then
            printf '%s' "${arg#--from-literal=status=}" > "$EKS_K8S/status.json"
        fi
    done
    exit 0
fi
if [[ "$args" == *"apply -f -"* ]]; then
    cat > /dev/null
    exit 0
fi
if [[ "$args" == *"get configmap gensim-run-status"* ]]; then
    cat "$EKS_K8S/status.json"
    exit 0
fi
if [[ "$args" == *"get pods"* && "$args" == *"-o json"* ]]; then
    count_file="$EKS_K8S/get-pods-count"
    c=0
    [[ -f "$count_file" ]] && c=$(cat "$count_file")
    c=$((c + 1))
    printf '%s' "$c" > "$count_file"
    if [[ "${EKS_FAIL_RESTART_AFTER:-0}" == "1" && "$c" -ge 2 ]]; then
        cat "$EKS_K8S/pods-restarted.json"
    else
        cat "$EKS_K8S/pods-healthy.json"
    fi
    exit 0
fi
if [[ "$args" == *"exec"* && "$args" == *"test -d"* ]]; then
    [[ "${EKS_FAIL_MISSING_DIR:-0}" == "0" ]] || exit 1
    exit 0
fi
if [[ "$args" == *"exec"* && "$args" == *"tar cf -"* ]]; then
    if [[ "$args" == *"datadog-agent-node2"* && "${EKS_FAIL_SECOND_POD_COPY:-0}" == "1" ]]; then
        exit 1
    fi
    if [[ "$args" == *"datadog-agent-node2"* ]]; then
        tar cf - -C "$EKS_ROOT/pods/datadog-agent-node2" .
    else
        tar cf - -C "$EKS_ROOT/pods/datadog-agent-node1" .
    fi
    exit 0
fi
SH

cat > "$eks_root/bin/aws" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'aws %s\n' "$*" >> "$EKS_CALLS"
if [[ "${1:-}" == "s3" && "${2:-}" == "cp" ]]; then
    src="$3"
    dst="$4"
    if [[ "$dst" == s3://* ]]; then
        [[ "${EKS_FAIL_S3_UPLOAD:-0}" == "0" ]] || exit 1
        cp -- "$src" "$EKS_S3/$(basename "$dst")"
    else
        key="$(basename "$src")"
        [[ -f "$EKS_S3/$key" ]] || exit 1
        cp -- "$EKS_S3/$key" "$dst"
    fi
fi
SH

cat > "$eks_root/bin/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl %s\n' "$*" >> "$EKS_CALLS"
cat > /dev/null || true
SH

chmod +x "$eks_root/bin/"*

run_orchestrator() {
    rm -f "$eks_root/k8s/get-pods-count" "$eks_root/k8s/status.json"
    rm -rf "$eks_root/work"
    mkdir -p "$eks_root/work"
    EKS_ROOT="$eks_root" \
    EKS_K8S="$eks_root/k8s" \
    EKS_S3="$eks_root/s3" \
    EKS_CALLS="$eks_root/calls.log" \
    GENSIM_WORKSPACE_ROOT="$eks_root/work" \
    GENSIM_EPISODES_MOUNT="$eks_root/episodes" \
    GENSIM_CONFIG_MOUNT="$eks_root/config" \
    EPISODES="${EKS_EPISODES:-authcore-pgbouncer:pool-saturation,authcore-pgbouncer:conn-spike}" \
    AGENT_IMAGE="docker.io/datadog/agent-dev:my-branch-recorder-full" \
    GENSIM_SHA="abcdef123456" \
    S3_BUCKET="qbranch-gensim-recordings" \
    KUBE_NAMESPACE="bench-ns" \
    IMAGE_REGISTRY="123456789012.dkr.ecr.us-east-1.amazonaws.com" \
    GENSIM_MODE="record-parquet" \
    RUN_ID="${EKS_RUN_ID:-eval-20261015T120000Z-abcdef1-112233}" \
    CLUSTER_NAME="gensim-bench" \
    RECORDING_OUTPUT_DIR="/tmp/observer-metrics" \
    RECORDING_FLUSH_INTERVAL="1s" \
    RECORDING_FLUSH_SECONDS="1" \
    RECORDING_RETENTION="90m" \
    RECORDING_ONLY="true" \
    DD_API_KEY="eks-secret-api-key" \
    DD_APP_KEY="eks-secret-app-key" \
    DD_SITE="datadoghq.com" \
    PATH="$eks_root/bin:$PATH" \
    bash "$orchestrator_script"
}

# 1. Happy path: 2 pods with identically named files + 2 scenarios from same episode dir + non-default namespace
run_orchestrator > "$eks_root/orchestrator-ok.log" 2>&1
if grep -q "eks-secret-api-key" "$eks_root/calls.log" || grep -q "eks-secret-app-key" "$eks_root/calls.log"; then
    echo "EKS secret leaked into command arguments" >&2
    exit 1
fi

python3 - "$eks_root/k8s/status.json" "$eks_root/s3/runs.jsonl" <<'PY'
import json
from pathlib import Path
import sys

status = json.loads(Path(sys.argv[1]).read_text())
assert status["overallStatus"] == "succeeded"
assert len(status["episodes"]) == 2
for ep in status["episodes"]:
    assert ep["status"] == "done"
    assert ep["episodeOutcome"] == "failure"
    assert ep["collectionStatus"] == "succeeded"
    assert ep["uploadStatus"] == "succeeded"
    assert ep["validationStatus"] == "pending"
    assert ep["recorderPods"] == 2
    assert ep["parquetFiles"] == 8

runs = [json.loads(line) for line in Path(sys.argv[2]).read_text().splitlines() if line.strip()]
assert len(runs) == 2
assert {r["scenario"] for r in runs} == {"pool-saturation", "conn-spike"}
assert len({r["zip"] for r in runs}) == 2
PY

# Validate the uploaded zip archive locally via validate-recording.sh
eks_out="$workspace/eks-fetched-pool-saturation"
mkdir -p "$eks_out/parquet"
cp "$eks_root/s3/gensim-results-eval-20261015T120000Z-abcdef1-112233-authcore-pgbouncer-pool-saturation.zip" "$eks_out/artifact.zip"
bash "$validator_script" "$eks_out" 60 --backend eks > /dev/null

python3 - "$eks_out" <<'PY'
import json
from pathlib import Path
import sys

output = Path(sys.argv[1])
report = json.loads((output / "validation.json").read_text())
manifest = json.loads((output / "recording-manifest.json").read_text())
assert report["usable"] is True
assert report["backend"] == "eks"
assert report["recorder_pods"] == 2
assert report["removed"] == 4
assert report["families"]["metrics"]["files"] == 2
assert report["families"]["metrics"]["rows"] == 8
assert report["families"]["logs"]["files"] == 2
assert report["families"]["logs"]["rows"] == 8
assert manifest["states"]["validation"] == "passed"
assert manifest["states"]["episode_outcome"] == "failure"  # monitor false in episode.json is distinct from recording validity
assert manifest["agent_history_verified"] is True
parquet_names = sorted(p.name for p in (output / "parquet").glob("*.parquet"))
assert parquet_names == [
    "observer-logs-20260101-000100Z-datadog-agent-node1.parquet",
    "observer-logs-20260101-000100Z-datadog-agent-node2.parquet",
    "observer-metrics-20260101-000100Z-datadog-agent-node1.parquet",
    "observer-metrics-20260101-000100Z-datadog-agent-node2.parquet",
]
assert (output / "parquet" / "datadog-agent-node1-advances.jsonl").exists()
assert (output / "parquet" / "datadog-agent-node2-advances.jsonl").exists()
PY

# 2. Failure cases in orchestrator: restarted pod, copy failure on second pod, S3 upload failure
if EKS_FAIL_RESTART_AFTER=1 EKS_EPISODES="authcore-pgbouncer:pool-saturation" EKS_RUN_ID="eval-restart" run_orchestrator > /dev/null 2>&1; then
    echo "orchestrator succeeded despite restarted pod" >&2
    exit 1
fi
python3 - "$eks_root/k8s/status.json" <<'PY'
import json, sys
st = json.loads(open(sys.argv[1]).read())
assert st["overallStatus"] == "failed"
assert st["episodes"][0]["status"] == "failed"
assert st["episodes"][0]["collectionStatus"] == "failed"
PY

if EKS_FAIL_SECOND_POD_COPY=1 EKS_EPISODES="authcore-pgbouncer:pool-saturation" EKS_RUN_ID="eval-copyfail" run_orchestrator > /dev/null 2>&1; then
    echo "orchestrator succeeded despite second pod copy failure" >&2
    exit 1
fi
python3 - "$eks_root/k8s/status.json" <<'PY'
import json, sys
st = json.loads(open(sys.argv[1]).read())
assert st["overallStatus"] == "failed"
assert st["episodes"][0]["status"] == "failed"
assert st["episodes"][0]["collectionStatus"] == "failed"
PY

if EKS_FAIL_S3_UPLOAD=1 EKS_EPISODES="authcore-pgbouncer:pool-saturation" EKS_RUN_ID="eval-s3fail" run_orchestrator > /dev/null 2>&1; then
    echo "orchestrator succeeded despite S3 upload failure" >&2
    exit 1
fi
python3 - "$eks_root/k8s/status.json" <<'PY'
import json, sys
st = json.loads(open(sys.argv[1]).read())
assert st["overallStatus"] == "failed"
assert st["episodes"][0]["status"] == "failed"
assert st["episodes"][0]["uploadStatus"] == "failed"
PY

# 3. Checksum mismatch and missing second-pod logs in validate-recording.sh --backend eks
bad_eks="$workspace/eks-bad-checksum"
mkdir -p "$bad_eks/parquet"
cp "$eks_root/s3/gensim-results-eval-20261015T120000Z-abcdef1-112233-authcore-pgbouncer-pool-saturation.zip" "$bad_eks/artifact.zip"
python3 - "$bad_eks/artifact.zip" <<'PY'
import json, sys, zipfile
from pathlib import Path
zp = Path(sys.argv[1])
with zipfile.ZipFile(zp, "r") as zin:
    items = {name: zin.read(name) for name in zin.namelist()}
manifest = json.loads(items["tmp/gensim-archive/recording-manifest.json"].decode())
manifest["files"][0]["sha256"] = "0" * 64
items["tmp/gensim-archive/recording-manifest.json"] = json.dumps(manifest).encode()
with zipfile.ZipFile(zp, "w") as zout:
    for name, content in items.items():
        zout.writestr(name, content)
PY
if bash "$validator_script" "$bad_eks" 60 --backend eks > /dev/null 2>&1; then
    echo "validator accepted checksum mismatch" >&2
    exit 1
fi
[[ -f "$bad_eks/validation.json" && -f "$bad_eks/artifact.zip" ]]

[[ "$(python3 "$scripts/recording.py" wait "$FIXTURE/episode.json" 60)" == 0 ]]
if [[ -x /usr/bin/python3 ]]; then
    [[ "$(/usr/bin/python3 "$scripts/recording.py" wait "$FIXTURE/episode.json" 60)" == 0 ]]
fi
if python3 "$scripts/recording.py" wait "$FIXTURE/episode.json" 0 > /dev/null 2>&1; then
    echo "invalid flush accepted" >&2; exit 1
fi

mkdir "$workspace/bootstrap-bin"
cat > "$workspace/bootstrap-bin/sudo" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$CALLS"
case "$1" in gpg|tee) cat >/dev/null ;; esac
SH
cat > "$workspace/bootstrap-bin/curl" <<'SH'
#!/usr/bin/env bash
printf 'fixture public key\n'
SH
cat > "$workspace/bootstrap-bin/grep" <<'SH'
#!/usr/bin/env bash
if [[ "${@: -1}" == /etc/os-release ]]; then exit 0; fi
exec /usr/bin/grep "$@"
SH
cat > "$workspace/bootstrap-bin/uname" <<'SH'
#!/usr/bin/env bash
echo x86_64
SH
cat > "$workspace/bootstrap-bin/lsb_release" <<'SH'
#!/usr/bin/env bash
echo jammy
SH
cat > "$workspace/bootstrap-bin/go" <<'SH'
#!/usr/bin/env bash
printf 'go %s\n' "$*" >> "$CALLS"
SH
cat > "$workspace/bootstrap-bin/docker" <<'SH'
#!/usr/bin/env bash
exit 0
SH
chmod +x "$workspace/bootstrap-bin/"*
mkdir "$workspace/bootstrap-home"
bootstrap() {
    HOME="$workspace/bootstrap-home" PATH="$workspace/bootstrap-bin:/usr/bin:/bin" bash "$scripts/bootstrap.sh" --disposable-vm
}
bootstrap
grep -q 'apt-get install .*docker-compose-plugin' "$CALLS"
grep -q 'apt-get install .*helm=3.20.2-1' "$CALLS"
grep -qx 'go install sigs.k8s.io/kind@v0.27.0' "$CALLS"
mv "$workspace/bootstrap-bin/go" "$workspace/bootstrap-bin/go-disabled"
bootstrap
grep -q 'curl .*kind.sigs.k8s.io/dl/v0.27.0/kind-linux-amd64' "$CALLS"
echo "Shell helper fixture tests passed"
