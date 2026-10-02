"""Build and install the feature-branch-only EUDM scenario simulator."""

import ipaddress
import json
import os
import plistlib
import re
import shutil
import ssl
import sys
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path

import yaml
from invoke import Exit, task

from tasks.build_tags import compute_build_tags_for_flavor
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.libs.common.constants import REPO_PATH
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import bin_name, get_build_flags, join_command


@task
@run_on_devcontainer
def build(ctx, build_include=None, build_exclude="python", rebuild=False, go_mod="readonly"):
    """Build eudm-simulator for live Agent capture or portable staging replay."""
    # Capture uses local Agent APIs; wire regeneration and replay do not embed Python.
    tags = compute_build_tags_for_flavor(
        flavor=AgentFlavor.base,
        build="agent",
        build_include=build_include,
        build_exclude=build_exclude,
    )
    ldflags, gcflags, env = get_build_flags(ctx, include_python="python" in tags)
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/eudm-simulator",
        build_tags=tags,
        ldflags=ldflags,
        gcflags=gcflags,
        env=env,
        rebuild=rebuild,
        mod=go_mod,
        bin_path=os.path.join("bin", "eudm-simulator", bin_name("eudm-simulator")),
    )


_MACOS_ROOT = Path("/opt/datadog-agent")
_MACOS_PLIST = Path("/Library/LaunchDaemons/com.datadoghq.agent.plist")
_MACOS_SERVICE = "system/com.datadoghq.agent"
_CAPTURE_TARGETS = {
    "agent": "//cmd/agent:agent",
    "process-agent": "//cmd/process-agent:process-agent",
    "eudm-simulator": "//cmd/eudm-simulator:eudm-simulator",
}
_CAPTURE_METRIC_FAMILIES = {"cpu", "memory", "disk", "uptime", "wlan", "battery", "network"}


def _checkout_commit(root):
    """Read checkout identity without running Git or its stamping hooks."""
    git_dir = root / ".git"
    try:
        if git_dir.is_file():
            prefix, path = git_dir.read_text().strip().split(": ", 1)
            if prefix != "gitdir":
                raise ValueError
            git_dir = (root / path).resolve()
        head = (git_dir / "HEAD").read_text().strip()
        if not head.startswith("ref: "):
            return head
        ref = head.removeprefix("ref: ")
        if not ref.startswith("refs/") or ".." in Path(ref).parts:
            raise ValueError
        common = git_dir
        if (git_dir / "commondir").exists():
            common = (git_dir / (git_dir / "commondir").read_text().strip()).resolve()
        for directory in (git_dir, common):
            loose = directory / ref
            if loose.is_file():
                return loose.read_text().strip()
        packed = common / "packed-refs"
        if packed.is_file():
            for line in packed.read_text().splitlines():
                fields = line.split()
                if len(fields) == 2 and fields[1] == ref:
                    return fields[0]
    except (OSError, ValueError):
        pass
    raise Exit("Cannot read checkout revision; supply --commit with the source's full 40-character commit.")


def _macos_preflight(root, plist):
    if sys.platform != "darwin":
        raise Exit("eudm-simulator.install currently supports macOS only; Windows installation is deferred.")
    if os.geteuid() == 0:
        raise Exit("Run dda inv eudm-simulator.install as your normal user; it requests sudo after building.")
    for name in ("bazel", "otool", "install_name_tool", "codesign", "sudo"):
        if shutil.which(name) is None:
            raise Exit(f"Required tool missing: {name}. See the repository's macOS development setup.")
    for relative in ("bin/agent/agent", "embedded/bin/process-agent", "embedded/lib/libdatadog-agent-rtloader.dylib"):
        if not (root / relative).is_file():
            raise Exit("An existing macOS Agent installation with its embedded Python runtime is required.")
    try:
        service = plistlib.loads(plist.read_bytes())
    except (OSError, ValueError, plistlib.InvalidFileException) as error:
        raise Exit("Cannot read the installed Agent launch daemon.") from error
    if service.get("Label") != "com.datadoghq.agent" or service.get("ProgramArguments") != [
        str(root / "bin/agent/agent"),
        "run",
    ]:
        raise Exit("This command supports the standard macOS Agent launch daemon only.")


def _build_capture_binaries(ctx, root, commit, race):
    flags = [
        "--nostamp",
        "--workspace_status_command=/usr/bin/true",
        "--lockfile_mode=error",
        "--noverbose_failures",
        f"--@rules_go//go/config:gc_linkopts=-X=github.com/DataDog/datadog-agent/pkg/version.FullCommit={commit}",
    ]
    if race:
        flags.append("--config=gorace")
    ctx.run(join_command(["bazel", "build", *flags, *_CAPTURE_TARGETS.values()]), echo=True)
    # dd_agent_go_binary transitions change output directories. Ask Bazel for the
    # actual files using the same flags instead of guessing bazel-bin paths.
    result = ctx.run(
        join_command(["bazel", "cquery", *flags, "--output=files", f"set({' '.join(_CAPTURE_TARGETS.values())})"]),
        hide="out",
    )
    binaries = {}
    for line in result.stdout.splitlines():
        path = root / line.strip()
        if path.name in _CAPTURE_TARGETS and path.is_file():
            if path.name in binaries:
                raise Exit("Bazel returned ambiguous capture binary outputs.")
            binaries[path.name] = path
    if binaries.keys() != _CAPTURE_TARGETS.keys():
        raise Exit("Bazel did not return all three capture binaries.")
    return binaries


def _stage_capture_binaries(ctx, binaries, staging, installed):
    for name, source in binaries.items():
        target = staging / name
        shutil.copyfile(source, target)
        target.chmod(0o755)
        if name == "agent":
            result = ctx.run(join_command(["otool", "-l", str(target)]), hide=True)
            # Replace build-tree paths with the existing installation's runtime.
            paths = re.findall(r"cmd LC_RPATH\s+cmdsize \d+\s+path (.*?) \(offset \d+\)", result.stdout)
            for path in paths:
                ctx.run(join_command(["install_name_tool", "-delete_rpath", path, str(target)]), hide=True)
            ctx.run(
                join_command(["install_name_tool", "-add_rpath", str(installed / "embedded/lib"), str(target)]),
                hide=True,
            )
        ctx.run(join_command(["codesign", "--force", "--sign", "-", str(target)]), hide=True)
        # Resolve lazy dylib symbols now, before modifying installed files. An
        # incompatible embedded runtime fails setup without replacing services.
        argument = "--help" if name == "eudm-simulator" else "version"
        ctx.run(join_command([str(target), argument]), env={"DYLD_BIND_AT_LAUNCH": "1"}, hide=True)


class _NoCaptureRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, _req, _fp, _code, _msg, _headers, _newurl):
        return None


def _capture_api_reader(installed):
    """Read existing local authentication in memory; never print status bodies."""
    try:
        config_path = installed / "etc/datadog.yaml"
        with config_path.open() as file:
            config = yaml.safe_load(file) or {}
        host = config["ipc_address"] if "ipc_address" in config else config.get("cmd_host", "localhost")
        if host != "localhost" and not ipaddress.ip_address(host).is_loopback:
            raise ValueError
        cert = config.get("ipc_cert_file_path") or installed / "etc/ipc_cert.pem"
        token = Path(config.get("auth_token_file_path") or installed / "etc/auth_token").read_text().strip()
        context = ssl.create_default_context(cafile=str(cert))
        ports = {
            "core-agent": int(config.get("cmd_port", 5001)),
            "process-agent": int(config.get("process_config", {}).get("cmd_port", 6162)),
        }
        if ports["process-agent"] <= 0:
            ports["process-agent"] = 6162
        if not token or any(port <= 0 or port > 65535 for port in ports.values()):
            raise ValueError
    except (OSError, ValueError, TypeError, AttributeError, yaml.YAMLError) as error:
        raise Exit(
            "Cannot read existing local API settings or IPC artifacts; check access to the installed configuration."
        ) from error
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=context), _NoCaptureRedirects()
    )
    address = f"[{host}]" if ":" in host else host

    def read(role):
        prefix = "/agent/eudm-capture" if role == "core-agent" else "/eudm-capture"
        request = urllib.request.Request(
            f"https://{address}:{ports[role]}{prefix}/capabilities", headers={"Authorization": "Bearer " + token}
        )
        with opener.open(request, timeout=3) as response:
            data = response.read(65537)
        if len(data) > 65536:
            raise ValueError
        return json.loads(data)

    return read


def _valid_metric_schedules(capability):
    """Match protocol-4 fixed check families and positive nanosecond durations."""
    schedules = capability.get("metric_schedules")
    if not isinstance(schedules, list) or not 1 <= len(schedules) <= len(_CAPTURE_METRIC_FAMILIES):
        return False
    seen = set()
    for schedule in schedules:
        if not isinstance(schedule, dict):
            return False
        family, cadence = schedule.get("family"), schedule.get("cadence")
        if (
            not isinstance(family, str)
            or family not in _CAPTURE_METRIC_FAMILIES
            or family in seen
            or not isinstance(cadence, int)
            or isinstance(cadence, bool)
            or not 0 < cadence <= 2**63 - 1
        ):
            return False
        seen.add(family)
    return True


def _wait_for_capture_apis(read, commit, timeout=120):
    required = {
        "core-agent": {"metrics", "metadata", "agent_inventory", "host_inventory", "software"},
        "process-agent": {"processes"},
    }
    deadline = time.monotonic() + timeout
    missing = set(required)
    while time.monotonic() < deadline:
        missing = set()
        hardware_available = False
        for role, streams in required.items():
            try:
                status = read(role)
                producer = status.get("producer", {})
                if (
                    status.get("protocol_version") != 4
                    or producer.get("role") != role
                    or producer.get("commit") != commit
                    or not producer.get("instance_id")
                ):
                    missing.add(role)
                    continue
                capabilities = status.get("capabilities", [])
                available = {item["stream"] for item in capabilities if item.get("cadence", 0) > 0}
                missing.update(f"{role}/{stream}" for stream in streams - available)
                if role == "core-agent":
                    hardware_available = "host_system_info" in available
                    metrics = [item for item in capabilities if item.get("stream") == "metrics"]
                    if len(metrics) != 1 or not _valid_metric_schedules(metrics[0]):
                        missing.add("core-agent/metric check cadences")
            except (OSError, ValueError, TypeError, KeyError, AttributeError):
                missing.add(role)
        if not missing:
            print(
                "Installed core and Process Agent expose capture protocol 4 with all required macOS streams "
                "and metric check cadences."
            )
            if hardware_available:
                print(
                    "Capture will collect fresh host system information once at startup; its regular hourly schedule stays unchanged."
                )
            return
        time.sleep(1)
    raise Exit(
        "Binaries were installed, but capture readiness timed out: "
        + ", ".join(sorted(missing))
        + ". Check Agent health and enabled streams; reinstall compatible producers if metric check cadences "
        "are missing or invalid. Configuration was not changed."
    )


@task(
    help={
        "prepare_only": "Build and check binaries without installing or requesting administrator access.",
        "race": "Build the simulator and producer binaries with the Go race detector.",
        "commit": "Full source commit for an exported tree; defaults to the checkout's HEAD metadata.",
    }
)
def install(ctx, prepare_only=False, race=False, commit=None):
    """Build and install compatible capture producers into an existing macOS Agent installation."""
    # Deliberately host-only: do not decorate with run_on_devcontainer.
    root = Path(__file__).resolve().parents[1]
    _macos_preflight(_MACOS_ROOT, _MACOS_PLIST)
    commit = commit or _checkout_commit(root)
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise Exit("The source commit must be exactly 40 lowercase hexadecimal characters.")
    read = _capture_api_reader(_MACOS_ROOT)
    with ctx.cd(str(root)):
        binaries = _build_capture_binaries(ctx, root, commit, race)
        output = root / "bin/eudm-simulator"
        output.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix="install-macos-", dir=output) as directory:
            staging = Path(directory)
            _stage_capture_binaries(ctx, binaries, staging, _MACOS_ROOT)
            os.replace(staging / "eudm-simulator", output / "eudm-simulator")
            if prepare_only:
                print("Build and runtime checks passed. No installed services changed.")
                print("Run dda inv eudm-simulator.install to install and restart the Agent.")
                return
            print(
                "Installing core and Process Agent, then restarting com.datadoghq.agent; configuration stays in place."
            )
            destinations = {
                "agent": _MACOS_ROOT / "bin/agent/agent",
                "process-agent": _MACOS_ROOT / "embedded/bin/process-agent",
            }
            commands = []
            for name, destination in destinations.items():
                commands.append(
                    join_command(
                        [
                            "/usr/bin/install",
                            "-o",
                            "root",
                            "-g",
                            "wheel",
                            "-m",
                            "0755",
                            str(staging / name),
                            str(destination) + ".eudm-new",
                        ]
                    )
                )
            for destination in destinations.values():
                commands.append(join_command(["/bin/mv", "-f", str(destination) + ".eudm-new", str(destination)]))
            commands.append(join_command(["/bin/launchctl", "kickstart", "-k", _MACOS_SERVICE]))
            script = staging / "install.sh"
            script.write_text("#!/bin/sh\nset -eu\n" + "\n".join(commands) + "\n")
            script.chmod(0o600)
            # One privilege request: a sudo ticket obtained on a temporary PTY
            # need not be valid for a subsequent non-PTY subprocess.
            ctx.run(join_command(["sudo", "/bin/sh", str(script)]), pty=True, echo=True)
    _wait_for_capture_apis(read, commit)
    print(
        "Capture with: ./bin/eudm-simulator/eudm-simulator capture --cfgpath /opt/datadog-agent/etc/datadog.yaml --duration 35m --output /private/tmp/eudm-macos-baseline"
    )
