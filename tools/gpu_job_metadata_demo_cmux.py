#!/usr/bin/env python3
"""Launch the GPU job metadata cmux demo.

Usage: tools/gpu_job_metadata_demo_cmux.py [--build]

There is intentionally only one knob:
  - macOS/non-Linux: run the lightweight placeholder check.
  - Linux: run the real gpu check with fake NVML and a tiny Docker workload.
  - --build: build the Agent first; on Linux also build fake NVML.
"""

from __future__ import annotations

import argparse
import json
import shlex
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path


API_KEY = "00000000000000000000000000000000"
AGENT_BIN = Path("bin/agent/agent")
DEMO_DIR = Path("/tmp/datadog-agent-gpu-job-metadata-demo")
FAKE_NVML_LIB = Path("bazel-bin/pkg/gpu/fake-nvml/libfake_nvml.so")

CONTAINER_NAME = "dd-gpu-job-demo"
PLACEHOLDER_CONTAINER_ID = "demo-container"
DOGSTATSD_PORT = 18125
CMD_PORT = 5501
INTERVAL = 5
WORKSPACE_NAME = "GPU job metadata demo"


@dataclass
class Runtime:
    fake_nvml: bool
    container_id: str
    fake_pid: int | None = None
    fake_nvml_lib: Path | None = None


def run(cmd: list[str], *, cwd: Path | None = None) -> None:
    subprocess.run(cmd, cwd=cwd, check=True, text=True)


def out(cmd: list[str], *, cwd: Path | None = None) -> str:
    return subprocess.check_output(cmd, cwd=cwd, text=True).strip()


def shell(script: str) -> str:
    return "bash -lc " + shlex.quote(script.strip())


def repo_root() -> Path:
    return Path(out(["git", "rev-parse", "--show-toplevel"])).resolve()


def is_linux() -> bool:
    return sys.platform == "linux"


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Launch the GPU job metadata cmux demo")
    parser.add_argument("--build", action="store_true", help="build the Agent first; on Linux also build fake NVML")
    return parser.parse_args(argv)


def build(root: Path) -> None:
    run(["dda", "inv", "agent.build", "--build-exclude=systemd"], cwd=root)
    if is_linux():
        run(["bazelisk", "build", "//pkg/gpu/fake-nvml:fake_nvml"], cwd=root)


def runtime(root: Path) -> Runtime:
    if not is_linux():
        return Runtime(fake_nvml=False, container_id=PLACEHOLDER_CONTAINER_ID)

    lib = (root / FAKE_NVML_LIB).resolve()
    if not lib.exists():
        raise RuntimeError(f"fake NVML library not found: {lib}\nRun this first: tools/gpu_job_metadata_demo_cmux.py --build")
    if shutil.which("docker") is None:
        raise RuntimeError("Docker is required for the Linux fake-NVML demo")

    # Use a real container so the real gpu check can exercise the normal
    # NVML process PID -> container ID -> tagger path.
    subprocess.run(["docker", "rm", "-f", CONTAINER_NAME], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    run(["docker", "run", "-d", "--rm", "--name", CONTAINER_NAME, "alpine", "sleep", "infinity"])
    cid = out(["docker", "inspect", "-f", "{{.Id}}", CONTAINER_NAME])
    pid = int(out(["docker", "inspect", "-f", "{{.State.Pid}}", CONTAINER_NAME]))
    print(f"Started demo container {CONTAINER_NAME}: cid={cid} pid={pid}")
    print(f"Clean up later with: docker rm -f {CONTAINER_NAME}")
    return Runtime(fake_nvml=True, container_id=cid, fake_pid=pid, fake_nvml_lib=lib)


def write_files(rt: Runtime) -> tuple[Path, Path]:
    demo_dir = DEMO_DIR.expanduser().resolve()
    shutil.rmtree(demo_dir, ignore_errors=True)

    confd = demo_dir / "conf.d"
    checks = demo_dir / "checks.d"
    check = "gpu" if rt.fake_nvml else "gpu_job_metadata_demo"
    check_dir = confd / f"{check}.d"
    check_dir.mkdir(parents=True)
    checks.mkdir(parents=True)

    config = demo_dir / "datadog.yaml"
    log = demo_dir / "agent.log"

    if rt.fake_nvml:
        gpu_config = f'''  enabled: true
  nvml_lib_path: "{rt.fake_nvml_lib}"
  disabled_collectors: [gpm, sampling, fields, ebpf, nvlink_plr, nvlink_fec, device_events]
  job_metadata:
    enabled: true
    ttl: 0s
'''
        log_level, log_payloads, series_payloads = "debug", "true", "true"
        check_config = f"instances:\n  - min_collection_interval: {INTERVAL}\n"
    else:
        gpu_config = '''  job_metadata:
    enabled: true
    ttl: 0s
'''
        log_level, log_payloads, series_payloads = "info", "false", "false"
        check_config = f'''init_config:

instances:
  - container_id: {rt.container_id}
    min_collection_interval: {INTERVAL}
    log_results: true
'''

    config.write_text(
        f'''api_key: "{API_KEY}"
site: datadoghq.com
dd_url: http://127.0.0.1:9
hostname: gpu-job-metadata-demo
log_level: {log_level}
log_payloads: {log_payloads}
log_file: "{log}"
cmd_port: {CMD_PORT}
confd_path: "{confd}"
additional_checksd: "{checks}"

dogstatsd_port: {DOGSTATSD_PORT}
dogstatsd_origin_detection_client: true
use_dogstatsd: true

remote_configuration:
  enabled: false
agent_telemetry:
  enabled: false
inventories_enabled: false
inventories_configuration_enabled: false
enable_payloads:
  events: false
  series: {series_payloads}
  service_checks: false
  sketches: false
  json_to_v1_intake: false

gpu:
{gpu_config}''',
        encoding="utf-8",
    )
    (check_dir / "conf.yaml").write_text(check_config, encoding="utf-8")
    return config, log


def layout(root: Path, rt: Runtime, config: Path, log: Path) -> dict:
    agent_bin = (root / AGENT_BIN).resolve()
    fake_env = ""
    if rt.fake_nvml:
        fake_env = f'''
export FAKE_NVML_DEVICE_COUNT=1
export FAKE_NVML_PROCESS_PID={rt.fake_pid}
echo "fake NVML: {rt.fake_nvml_lib}"
echo "fake GPU process PID: $FAKE_NVML_PROCESS_PID"
'''

    agent = f'''
set -euo pipefail
printf '\\033]0;GPU job metadata demo: Agent\\007'
cd {shlex.quote(str(root))}
echo "config: {config}"
echo "log:    {log}"
{fake_env}
if [ ! -x {shlex.quote(str(agent_bin))} ]; then
  echo "Agent binary not found: {agent_bin}"
  echo "Build it with: tools/gpu_job_metadata_demo_cmux.py --build"
  exit 1
fi
exec {shlex.quote(str(agent_bin))} run -c {shlex.quote(str(config))}
'''

    hint = "gpu.process.memory.usage ... gpu_job_id" if rt.fake_nvml else "gpu_job_metadata_demo emitted ... gpu_job_id"
    wait = 15 if rt.fake_nvml else 12
    sender = f'''
set -euo pipefail
printf '\\033]0;GPU job metadata demo: sender\\007'
echo "Sending reserved DogStatsD events for c:ci-{rt.container_id}"
echo "Watch logs for: DogStatsD GPU job metadata published/cleared and {hint}"
python3 - <<'PY'
import socket, time
host, port = "127.0.0.1", {DOGSTATSD_PORT}
container_id = {rt.container_id!r}

def send(action, job=None, phase=None):
    tags = []
    if job:
        tags.append(f"gpu_job_id:{{job}}")
    if phase:
        tags += ["team:ml", f"phase:{{phase}}"]
    msg = "_e{{15,%d}}:datadog.gpu.job|%s|s:datadog_gpu_job|c:ci-%s" % (len(action), action, container_id)
    if tags:
        msg += "|#" + ",".join(tags)
    print("sending ->", msg, flush=True)
    socket.socket(socket.AF_INET, socket.SOCK_DGRAM).sendto((msg + "\\n").encode(), (host, port))

time.sleep({wait})
send("start", "job-123", "first")
time.sleep({INTERVAL * 2})
send("end")
time.sleep({INTERVAL})
send("start", "job-456", "second")
print("done", flush=True)
PY
exec "${{SHELL:-/bin/bash}}"
'''

    pattern = "GPU job metadata|gpu_job_metadata_demo"
    if rt.fake_nvml:
        pattern = "GPU job metadata|gpu_job_id|gpu\\.process\\.memory\\.usage|gpu\\.memory\\.limit"
    logs = f'''
set -euo pipefail
printf '\\033]0;GPU job metadata demo: logs\\007'
touch {shlex.quote(str(log))}
echo "Watching {log}"
tail -n +1 -F {shlex.quote(str(log))} | awk '/{pattern}/ {{ print; fflush() }}'
'''

    def pane(script: str) -> dict:
        return {"pane": {"surfaces": [{"type": "terminal", "command": shell(script)}]}}

    return {
        "direction": "horizontal",
        "split": 0.58,
        "children": [pane(agent), {"direction": "vertical", "split": 0.5, "children": [pane(sender), pane(logs)]}],
    }


def main_impl(argv: list[str]) -> int:
    args = parse_args(argv)
    root = repo_root()
    if shutil.which("cmux") is None:
        raise RuntimeError("cmux was not found on PATH")
    if args.build:
        build(root)

    rt = runtime(root)
    config, log = write_files(rt)
    mode = f"real gpu check with fake NVML ({rt.fake_nvml_lib})" if rt.fake_nvml else "lightweight placeholder check"
    print(f"Generated demo config: {config}")
    print(f"Mode: {mode}")
    run(
        [
            "cmux",
            "new-workspace",
            "--name",
            WORKSPACE_NAME,
            "--cwd",
            str(root),
            "--layout",
            json.dumps(layout(root, rt, config, log)),
            "--focus",
            "true",
        ],
        cwd=root,
    )
    return 0


def main(argv: list[str]) -> int:
    try:
        return main_impl(argv)
    except RuntimeError as err:
        print(err, file=sys.stderr)
        return 1
    except subprocess.CalledProcessError as err:
        cmd = err.cmd if isinstance(err.cmd, list) else [str(err.cmd)]
        print(f"command failed ({err.returncode}): {' '.join(shlex.quote(str(part)) for part in cmd)}", file=sys.stderr)
        return err.returncode


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
