"""Local macOS Agent Sandbox tasks."""

from __future__ import annotations

import json
import shlex
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


@task
def build_helper(ctx, state_root=None, force=False):
    """Build the macOS Virtualization.framework helper."""
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
        print(f"Helper already up to date: {output}")
        return
    result = ctx.run(shlex.join(["swiftc", str(source), "-o", str(output)]), warn=True)
    if result.exited != 0:
        raise Exit(code=result.exited)
    sign = ctx.run(
        shlex.join(["codesign", "--force", "--sign", "-", "--entitlements", str(entitlements), str(output)]),
        warn=True,
    )
    if sign.exited != 0:
        raise Exit(code=sign.exited)
    print(f"Built {output}")


@task(name="prepare-base")
def prepare_base(ctx, name="base-builder", state_root=None, helper_path=None):
    """Build a prepared Ubuntu base image with OS dependencies prebaked."""
    manager = _manager(state_root, helper_path)
    try:
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
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def create(
    ctx,
    name=DEFAULT_SANDBOX_NAME,
    agent_version=None,
    config=None,
    ubuntu_image=None,
    state_root=None,
    helper_path=None,
    prepare_only=False,
    wait_agent=True,
    fx_trace=False,
):
    """Create local state for a host Agent sandbox."""
    manager = _manager(state_root, helper_path)
    try:
        manager.prepare_host_sandbox(
            name=name,
            agent_version=agent_version,
            config=Path(config) if config else None,
            ubuntu_image=Path(ubuntu_image) if ubuntu_image else None,
            fx_trace=fx_trace,
        )
        print(f"Prepared host Agent sandbox {name!r} in {manager.paths(name).instance_dir}")
        print(f"Managed SSH public key: {manager.paths(name).public_key}")
        print(f"Host provisioning script: {manager.paths(name).host_install_script}")
        if prepare_only:
            return
        _run_or_raise(ctx, manager.helper_command(name, "validate"))
        metadata = manager.start_background(name)
        print(f"Started sandbox {name!r} with helper pid {metadata.vm_pid}")
        endpoint = manager.discover_ssh_endpoint(name)
        print(f"SSH endpoint: {endpoint.ssh_host}:{endpoint.ssh_port}")
        if wait_agent:
            manager.wait_agent_ready(name)
            print("Agent command port is ready")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task(name="validate-vm")
def validate_vm(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None, helper_path=None):
    """Validate the VM helper configuration for a prepared sandbox."""
    manager = _manager(state_root, helper_path)
    try:
        _run_or_raise(ctx, manager.helper_command(name, "validate"))
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def start(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None, helper_path=None, wait_for_ssh=True, wait_agent=True):
    """Start the prepared sandbox VM through the macOS helper."""
    manager = _manager(state_root, helper_path)
    try:
        metadata = manager.start_background(name)
        print(f"Started sandbox {name!r} with helper pid {metadata.vm_pid}")
        if wait_for_ssh:
            endpoint = manager.discover_ssh_endpoint(name)
            print(f"SSH endpoint: {endpoint.ssh_host}:{endpoint.ssh_port}")
            if wait_agent:
                manager.wait_agent_ready(name)
                print("Agent command port is ready")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def stop(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Stop a running sandbox VM."""
    manager = _manager(state_root)
    try:
        metadata = manager.stop(name)
        print(f"Stopped sandbox {metadata.name!r}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def status(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Print sandbox metadata and local state paths."""
    manager = _manager(state_root)
    try:
        print(json.dumps(manager.status(name), indent=2, sort_keys=True))
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task(name="discover-ssh")
def discover_ssh(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None, timeout_seconds=180):
    """Discover and record the SSH endpoint for a running sandbox."""
    manager = _manager(state_root)
    try:
        endpoint = manager.discover_ssh_endpoint(name, int(timeout_seconds))
        print(f"SSH endpoint: {endpoint.ssh_host}:{endpoint.ssh_port}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task(name="set-ssh-endpoint")
def set_ssh_endpoint(ctx, host, port, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Record the managed SSH endpoint for a prepared sandbox."""
    manager = _manager(state_root)
    try:
        metadata = manager.update_connection(name=name, ssh_host=host, ssh_port=int(port))
        print(f"Sandbox {metadata.name!r} SSH endpoint set to {host}:{port}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def ssh(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Open direct SSH access using managed sandbox credentials."""
    manager = _manager(state_root)
    try:
        _run_or_raise(ctx, manager.ssh_command(name))
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def agent(ctx, args="status", name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Run a Datadog Agent command through managed SSH."""
    manager = _manager(state_root)
    try:
        _run_or_raise(ctx, manager.agent_command(name, args))
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task(name="fx-spans")
def fx_spans(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Print captured FX trace spans from a sandbox created with --fx-trace."""
    manager = _manager(state_root)
    try:
        _run_or_raise(ctx, manager.ssh_command(name, ["sudo", "cat", "/var/log/datadog/fx-trace-spans.jsonl"]))
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def logs(ctx, name=DEFAULT_SANDBOX_NAME, lines=200, state_root=None):
    """Show recent Datadog Agent service logs through managed SSH."""
    manager = _manager(state_root)
    try:
        _run_or_raise(ctx, manager.logs_command(name, int(lines)))
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None


@task
def destroy(ctx, name=DEFAULT_SANDBOX_NAME, state_root=None):
    """Destroy sandbox instance state while preserving cached base images."""
    manager = _manager(state_root)
    try:
        removed = manager.destroy(name)
        print(f"Removed sandbox state: {removed}")
    except AgentSandboxError as e:
        raise Exit(message=str(e), code=1) from None
