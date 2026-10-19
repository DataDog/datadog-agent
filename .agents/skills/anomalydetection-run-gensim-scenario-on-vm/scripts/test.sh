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
bash "$scripts/vm.sh" deploy-recorder --disposable-vm user@vm "$workspace/env" test-tag skill-test 1m 90m true
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
