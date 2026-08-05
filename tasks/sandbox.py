"""Local macOS Agent Sandbox tasks."""

from __future__ import annotations

import json
import os
import platform
import shlex
import shutil
import subprocess
import time
from pathlib import Path

from invoke import Exit, task

from tasks.libs.agent_sandbox.manager import DEFAULT_SANDBOX_NAME, AgentSandboxError, AgentSandboxManager


def _manager(state_root=None, helper_path=None) -> AgentSandboxManager:
    return AgentSandboxManager(state_root=state_root or AgentSandboxManager().state_root, helper_path=helper_path)


def _run_or_raise(ctx, command: list[str]) -> None:
    printable = shlex.join(command)
    result = ctx.run(printable, pty=True, warn=True)
    if result.exited != 0:
        raise Exit(code=result.exited)


def _human_path(path: Path) -> str:
    try:
        return str(path.expanduser()).replace(str(Path.home()), "~", 1)
    except Exception:
        return str(path)


def _size(path: Path) -> str:
    if not path.exists():
        return "missing"
    total = 0
    if path.is_file():
        total = path.stat().st_size
    else:
        for child in path.rglob("*"):
            if child.is_file():
                total += child.stat().st_size
    for unit in ("B", "KB", "MB", "GB"):
        if total < 1024 or unit == "GB":
            return f"{total:.1f}{unit}" if unit != "B" else f"{total}B"
        total /= 1024
    return f"{total:.1f}GB"


def _ensure_helper(ctx, manager: AgentSandboxManager) -> None:
    dev_build_helper(ctx, state_root=str(manager.state_root))


def _prepare_base(ctx, manager: AgentSandboxManager, name="base-builder", force=False) -> Path:
    prepared = manager.prepared_base_path()
    if prepared.exists() and not force:
        print(f"Prepared base already exists: {prepared}")
        return prepared
    if prepared.exists():
        prepared.unlink()
    manager.prepare_base_builder(name=name)
    _run_or_raise(ctx, manager.helper_command(name, "validate"))
    metadata = manager.start_background(name)
    print(f"Started base builder {name!r} with helper pid {metadata.vm_pid}")
    endpoint = manager.discover_ssh_endpoint(name)
    print(f"SSH endpoint: {endpoint.ssh_host}:{endpoint.ssh_port}")
    _run_or_raise(ctx, manager.ssh_command(name, ["cloud-init", "status", "--wait"]))
    prepared = manager.finalize_prepared_base(name)
    print(f"Prepared base image: {prepared}")
    manager.destroy(name)
    return prepared


def _ensure_prepared_base(ctx, manager: AgentSandboxManager) -> None:
    if manager.prepared_base_path().exists():
        return
    _prepare_base(ctx, manager)


def _ensure_fakeintake(ctx, manager: AgentSandboxManager) -> None:
    if manager.fakeintake_binary().exists():
        return
    bazel = shutil.which("bazelisk") or shutil.which("bazel")
    if bazel:
        result = ctx.run(shlex.join([bazel, "build", "//test/fakeintake/cmd/server:server"]), warn=True, pty=True)
        if result.exited == 0 or manager.fakeintake_binary().exists():
            return
    go125 = Path("/opt/homebrew/opt/go@1.25/bin")
    if go125.exists():
        result = ctx.run(
            "PATH=" + shlex.quote(str(go125)) + ':$PATH dda inv fakeintake.build',
            warn=True,
            pty=True,
        )
        if result.exited == 0 or manager.fakeintake_binary().exists():
            return
    raise Exit(message="failed to build fakeintake with bazel/bazelisk or local Go 1.25", code=1)


@task
def up(
    ctx,
    name=DEFAULT_SANDBOX_NAME,
    agent_version=None,
    config=None,
    ubuntu_image=None,
    state_root=None,
    helper_path=None,
    fx_trace=False,
    kubernetes=False,
    agent_image="gcr.io/datadoghq/agent:7",
    values=None,
    wait_agent=True,
):
    """Create/start the sandbox and wait until the Agent is ready."""
    manager = _manager(state_root, helper_path)
    print(f"Sandbox: {name}")
    print(f"State root: {_human_path(manager.state_root)}")
    print("\nPreparing sandbox...")
    try:
        _ensure_helper(ctx, manager)
        print("✓ helper ready")
        if not ubuntu_image:
            _ensure_prepared_base(ctx, manager)
            print("✓ prepared Ubuntu base ready")

        paths = manager.paths(name)
        fakeintake_url = None
        fakeintake_pid = None
        fakeintake_log = None
        if not paths.metadata_file.exists() and not config:
            _ensure_fakeintake(ctx, manager)
            fakeintake_url, fakeintake_pid, fakeintake_log = manager.start_fakeintake_process(name)
            print(f"✓ fakeintake ready: {fakeintake_url}")
        if not paths.metadata_file.exists():
            manager.prepare_host_sandbox(
                name=name,
                agent_version=agent_version,
                config=Path(config) if config else None,
                ubuntu_image=Path(ubuntu_image) if ubuntu_image else None,
                fx_trace=fx_trace,
                kubernetes=kubernetes,
                agent_image=agent_image if kubernetes else None,
                helm_values_source=str(Path(values).expanduser()) if values else None,
                fakeintake_url=fakeintake_url,
                fakeintake_pid=fakeintake_pid,
                fakeintake_log=fakeintake_log,
            )
            print("✓ VM created")
        else:
            print("✓ VM state exists")

        _run_or_raise(ctx, manager.helper_command(name, "validate"))
        metadata = manager.read_metadata(name)
        if not metadata.vm_pid or not manager.pid_is_running(metadata.vm_pid):
            metadata = manager.start_background(name)
            print(f"✓ VM started (pid {metadata.vm_pid})")
        else:
            print(f"✓ VM already running (pid {metadata.vm_pid})")

        if not metadata.ssh_host or not manager.tcp_port_open(metadata.ssh_host, metadata.ssh_port or 22):
            try:
                metadata = manager.discover_ssh_endpoint(name)
            except AgentSandboxError:
                recovered_host = manager.ip_for_mac(metadata.mac_address or "")
                if not recovered_host:
                    raise
                metadata = manager.update_connection(name, recovered_host, 22, state="running")
        print(f"✓ SSH ready: {metadata.ssh_host}:{metadata.ssh_port}")

        if kubernetes:
            metadata = manager.provision_kubernetes(
                name, agent_image=agent_image, helm_values=Path(values) if values else None
            )
            print(f"✓ Kubernetes ready ({metadata.k3s_version})")
            print(f"✓ Agent image: {metadata.agent_image}")
            print(
                f"✓ Kubeconfig: {_human_path(Path(metadata.kubeconfig_path)) if metadata.kubeconfig_path else 'unknown'}"
            )
        elif wait_agent:
            manager.wait_agent_ready(name)
            print("✓ Agent status API ready")

        version = subprocess.run(
            manager.agent_command(name, "version"),
            check=False,
            text=True,
            capture_output=True,
        )
        if version.returncode == 0:
            print(f"✓ Agent: {version.stdout.strip()}")

        print("\nNext:")
        print(f"  dda inv sandbox.status --name {name}")
        print(f"  dda inv sandbox.ssh --name {name}")
        if kubernetes:
            print(f"  dda inv sandbox.kubeconfig --name {name}")
            print(f"  KUBECONFIG={manager.paths(name).instance_dir / 'kubeconfig'} kubectl get pods -A")
        else:
            print(f"  dda inv sandbox.ssh --name {name} --cmd 'sudo agent status'")
        print(f"  dda inv sandbox.logs --name {name}")
        print(f"  dda inv sandbox.down --name {name}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def status(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None, json_output=False):
    """Show sandbox status and useful next commands."""
    manager = _manager(state_root)
    try:
        data = manager.status(name)
        if json_output:
            print(json.dumps(data, indent=2, sort_keys=True))
            return
        print(f"Sandbox: {name}")
        print(f"Mode: {data.get('mode')}")
        print(f"State: {data['state']}")
        print(f"State root: {_human_path(Path(data['state_root']))}")
        print(f"Instance: {_human_path(Path(data['instance_dir']))}")
        print(f"SSH: {data.get('ssh_host') or 'unknown'}:{data.get('ssh_port') or 'unknown'}")
        print(f"Agent version request: {data.get('agent_version') or 'default published version'}")
        print(f"Base image: {'prepared' if (manager.prepared_base_path()).exists() else 'raw'}")
        print(f"Apt cache: {_size(manager.paths(name).apt_cache_dir)}")
        print(f"Fx tracing: {'enabled' if data.get('fx_trace') else 'disabled'}")
        print(f"Fakeintake: {data.get('fakeintake_url') or 'disabled/custom config'}")
        if data.get("kubernetes"):
            print("Kubernetes: enabled")
            print(f"Agent image: {data.get('agent_image') or 'unknown'}")
            print(
                f"Kubeconfig: {_human_path(Path(data['kubeconfig_path'])) if data.get('kubeconfig_path') else 'not exported'}"
            )
            _run_or_raise(ctx, manager.kubectl_command(name, "get nodes -o wide"))
            _run_or_raise(ctx, manager.kubectl_command(name, "-n datadog get pods -o wide"))
        print("\nUseful:")
        print(f"  dda inv sandbox.ssh --name {name}")
        if data.get("kubernetes"):
            print(f"  dda inv sandbox.kubeconfig --name {name}")
            print(f"  KUBECONFIG={data.get('kubeconfig_path')} kubectl get pods -A")
        else:
            print(f"  dda inv sandbox.ssh --name {name} --cmd 'sudo agent status'")
        print(f"  dda inv sandbox.logs --name {name}")
        print(f"  dda inv sandbox.down --name {name}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def ssh(ctx, name=DEFAULT_SANDBOX_NAME, cmd=None, state_root=None):
    """Open SSH or run a command inside the sandbox with managed credentials."""
    manager = _manager(state_root)
    try:
        if cmd:
            print(f"Sandbox: {name}")
            print(f"$ {cmd}\n")
            _run_or_raise(ctx, manager.shell_command(name, cmd))

        else:
            command = manager.ssh_command(name)
            os.execvp(command[0], command)
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def logs(ctx, name=DEFAULT_SANDBOX_NAME, lines=200, state_root=None):
    """Show recent Datadog Agent service logs from the sandbox."""
    manager = _manager(state_root)
    try:
        metadata = manager.read_metadata(name)
        if metadata.kubernetes:
            _run_or_raise(
                ctx,
                manager.kubectl_command(
                    name, f"-n datadog logs -l app=datadog-agent --tail {int(lines)} --all-containers=true"
                ),
            )
        else:
            _run_or_raise(ctx, manager.logs_command(name, int(lines)))
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def kubeconfig(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Export/print the host-usable kubeconfig path for a Kubernetes sandbox."""
    manager = _manager(state_root)
    try:
        path = manager.export_kubeconfig(name)
        metadata = manager.read_metadata(name)
        metadata.kubeconfig_path = str(path)
        manager.write_metadata(manager.paths(name).metadata_file, metadata)
        print(_human_path(path))
        print(f"export KUBECONFIG={path}")
    except (AgentSandboxError, subprocess.CalledProcessError) as e:
        raise Exit(message=str(e), code=1) from None


@task
def down(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None, stop_only=False):
    """Stop and destroy sandbox instance state while preserving caches."""
    manager = _manager(state_root)
    try:
        if stop_only:
            metadata = manager.stop(name)
            print(f"Stopped sandbox {metadata.name!r}; instance state preserved")
            return
        removed = manager.destroy(name)
        print("Sandbox removed")
        print(f"Removed: {_human_path(removed)}")
        print(f"Preserved cache: {_human_path(manager.paths(name).cache_dir)}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def doctor(ctx, state_root=None):
    """Check host prerequisites, caches, and running sandbox helpers."""
    manager = _manager(state_root)
    paths = manager.paths()
    print("Host:")
    print(f"{'✓' if platform.system() == 'Darwin' else '✗'} macOS")
    print(f"{'✓' if platform.machine() == 'arm64' else '✗'} Apple Silicon")
    print(
        f"{'✓' if Path('/System/Library/Frameworks/Virtualization.framework').exists() else '✗'} Virtualization.framework"
    )
    for tool in ("swiftc", "codesign", "qemu-img", "hdiutil", "ssh", "ssh-keygen"):
        print(f"{'✓' if shutil.which(tool) else '✗'} {tool}")
    print("\nCache:")
    print(
        f"helper: {_human_path(manager.state_root / 'bin' / 'agent-sandbox-vz') if (manager.state_root / 'bin' / 'agent-sandbox-vz').exists() else 'missing'}"
    )
    print(f"prepared base: {_size(manager.prepared_base_path())}")
    print(f"apt archives: {_size(paths.apt_cache_dir)}")
    print("\nRunning helpers:")
    result = subprocess.run(["pgrep", "-fl", "agent-sandbox-vz"], check=False, text=True, capture_output=True)
    print(result.stdout.strip() or "none")


@task(name="cache-status")
def cache_status(ctx, state_root=None):
    """Show sandbox cache state."""
    manager = _manager(state_root)
    paths = manager.paths()
    print(f"Cache root: {_human_path(paths.cache_dir)}")
    print(f"Ubuntu source image: {_size(paths.cache_dir / 'ubuntu-noble-arm64.img')}")
    print(f"Ubuntu raw base: {_size(paths.cache_dir / 'ubuntu-noble-arm64.raw')}")
    print(f"Prepared base: {_size(manager.prepared_base_path())}")
    print(f"Apt archives: {_size(paths.apt_cache_dir)}")
    print(
        f"Helper: {_human_path(manager.state_root / 'bin' / 'agent-sandbox-vz') if (manager.state_root / 'bin' / 'agent-sandbox-vz').exists() else 'missing'}"
    )


@task(name="cache-prepare")
def cache_prepare(ctx, state_root=None, helper=True, base=True, force=False):
    """Prepare helper and base-image caches."""
    manager = _manager(state_root)
    print("Preparing sandbox cache...")
    if helper:
        dev_build_helper(ctx, state_root=str(manager.state_root), force=force)
        print("✓ helper ready")
    if base:
        _prepare_base(ctx, manager, force=force)
        print("✓ prepared Ubuntu base ready")
    print(f"Cache root: {_human_path(manager.paths().cache_dir)}")


@task(name="cache-clear")
def cache_clear(ctx, state_root=None, base=False, apt=False, all=False):
    """Clear selected sandbox caches."""
    manager = _manager(state_root)
    paths = manager.paths()
    if not any((base, apt, all)):
        raise Exit(message="Choose what to clear: --base, --apt, or --all", code=1)
    if all or base:
        for path in (
            paths.cache_dir / "ubuntu-noble-arm64.img",
            paths.cache_dir / "ubuntu-noble-arm64.raw",
            manager.prepared_base_path(),
        ):
            if path.exists():
                path.unlink()
                print(f"Removed {path}")
    if all or apt:
        if paths.apt_cache_dir.exists():
            shutil.rmtree(paths.apt_cache_dir)
            print(f"Removed {paths.apt_cache_dir}")


@task(name="fx-spans")
def fx_spans(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None, summary=False):
    """Print captured FX trace spans from a sandbox created with --fx-trace."""
    manager = _manager(state_root)
    try:
        command = manager.ssh_command(name, ["sudo", "cat", "/var/log/datadog/fx-trace-spans.jsonl"])
        if not summary:
            _run_or_raise(ctx, command)
            return
        result = subprocess.run(command, check=True, text=True, capture_output=True)
        spans = []
        for line in result.stdout.splitlines():
            line = line.strip()
            if not line or not line[0].isdigit():
                continue
            _, payload = line.split(" ", 1)
            traces = json.loads(payload)
            for trace in traces:
                spans.extend(trace)
        spans.sort(key=lambda span: span.get("duration", 0), reverse=True)
        print(f"span_count {len(spans)}")
        for span in spans[:40]:
            duration = span.get("duration", 0) / 1e9
            print(f"{duration:8.3f}s {span.get('name')} {span.get('resource')}")
    except (AgentSandboxError, subprocess.CalledProcessError) as e:
        raise Exit(message=str(e), code=1) from None


@task(name="install-timeline")
def install_timeline(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Show sandbox installer timeline markers."""
    manager = _manager(state_root)
    try:
        _run_or_raise(
            ctx, manager.ssh_command(name, ["sudo", "cat", "/var/log/datadog/agent-sandbox-install-timeline.log"])
        )
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def benchmark(ctx, state_root=None, name="bench", prepare_base_first=True, fx_trace=False, granular=False):
    """Run a local end-to-end Stage A benchmark."""
    manager = _manager(state_root)
    root = str(manager.state_root)
    if prepare_base_first:
        cache_prepare(ctx, state_root=root)
    start_time = time.time()

    def mark(label):
        print(f"{label} {int(time.time() - start_time)}s")

    def wait_for(label, command, timeout=180):
        deadline = time.time() + timeout
        last = ""
        while time.time() < deadline:
            result = subprocess.run(command, check=False, text=True, capture_output=True)
            if result.returncode == 0:
                mark(label)
                return
            last = (result.stdout + result.stderr).strip()
            time.sleep(1)
        raise Exit(message=f"timed out waiting for {label}: {last}", code=1)

    mark("start")
    if not granular:
        up(ctx, name=name, state_root=root, fx_trace=fx_trace)
        mark("agent_status_ready")
        ssh(ctx, name=name, state_root=root, cmd="sudo agent version")
        mark("agent_version_done")
        down(ctx, name=name, state_root=root)
        mark("destroy_done")
        return

    up(ctx, name=name, state_root=root, fx_trace=fx_trace, wait_agent=False)
    mark("up_no_wait_returned")
    wait_for("agent_binary_present", manager.shell_command(name, "test -x /opt/datadog-agent/bin/agent/agent"))
    wait_for("cloud_init_done", manager.shell_command(name, "cloud-init status 2>/dev/null | grep -q 'status: done'"))
    wait_for("service_active", manager.shell_command(name, "systemctl is-active --quiet datadog-agent"))
    wait_for("cmd_api_port_open", manager.shell_command(name, "timeout 1 bash -lc '</dev/tcp/127.0.0.1/5001'"))
    wait_for(
        "cmd_api_log_seen",
        manager.shell_command(name, "sudo journalctl -u datadog-agent --no-pager | grep -q \"CMD API Server\""),
    )
    rc_seen = subprocess.run(
        manager.shell_command(
            name, "sudo journalctl -u datadog-agent --no-pager | grep -q \"first update successful\""
        ),
        check=False,
        text=True,
        capture_output=True,
    )
    if rc_seen.returncode == 0:
        mark("remote_config_first_update")
    else:
        mark("remote_config_disabled_or_no_update")
    wait_for("agent_status_ready", manager.agent_command(name, "status"))
    install_timeline(ctx, name=name, state_root=root)
    down(ctx, name=name, state_root=root)
    mark("destroy_done")


@task(name="dev-build-helper")
def dev_build_helper(ctx, state_root=None, force=False):
    """Internal: build the macOS Virtualization.framework helper."""
    manager = _manager(state_root)
    output = manager.state_root / "bin" / "agent-sandbox-vz"
    output.parent.mkdir(parents=True, exist_ok=True)
    source = Path("tools/agent-sandbox/vz-helper.swift")
    entitlements = Path("tools/agent-sandbox/vz-helper.entitlements")
    if (
        not force
        and output.exists()
        and output.stat().st_mtime >= max(source.stat().st_mtime, entitlements.stat().st_mtime)
    ):
        print(f"Virtualization helper is up to date: {_human_path(output)}")
        return
    result = ctx.run(shlex.join(["swiftc", str(source), "-o", str(output)]), warn=True, hide=True)
    if result.exited != 0:
        print(result.stdout)
        print(result.stderr)
        raise Exit(code=result.exited)
    sign = ctx.run(
        shlex.join(["codesign", "--force", "--sign", "-", "--entitlements", str(entitlements), str(output)]),
        warn=True,
        hide=True,
    )
    if sign.exited != 0:
        print(sign.stdout)
        print(sign.stderr)
        raise Exit(code=sign.exited)
    print(f"Built Virtualization helper: {_human_path(output)}")


@task(name="dev-validate-vm")
def dev_validate_vm(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None, helper_path=None):
    """Internal: validate the VM helper configuration for an existing sandbox."""
    manager = _manager(state_root, helper_path)
    try:
        _run_or_raise(ctx, manager.helper_command(name, "validate"))
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task(name="dev-discover-ssh")
def dev_discover_ssh(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None, timeout_seconds=180):
    """Internal: discover and record the SSH endpoint for a running sandbox."""
    manager = _manager(state_root)
    try:
        endpoint = manager.discover_ssh_endpoint(name, int(timeout_seconds))
        print(f"SSH endpoint: {endpoint.ssh_host}:{endpoint.ssh_port}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task(name="dev-set-ssh-endpoint")
def dev_set_ssh_endpoint(ctx, host, port, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Internal: record the managed SSH endpoint for a prepared sandbox."""
    manager = _manager(state_root)
    try:
        metadata = manager.update_connection(name=name, ssh_host=host, ssh_port=int(port))
        print(f"Sandbox {metadata.name!r} SSH endpoint set to {host}:{port}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None
