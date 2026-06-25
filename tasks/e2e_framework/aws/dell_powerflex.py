"""
Invoke tasks for the Dell PowerFlex all-in-one lab (scenario aws/dell-powerflex).

Single framework-provisioned m5.metal RHEL9 host that runs the libvirt
virtualization stack, the nested PFMP/MDM/SDS cluster (brought up during live
exploration), and the released Datadog Agent + dell_powerflex check.

The host is exported as dd-Host-powerflex. The aws namer prefixes "aws-", so the
role string passed to exec/ssh is "aws-powerflex".
"""

import shlex

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy
from tasks.e2e_framework.tool import add_known_host as add_known_host_func
from tasks.e2e_framework.tool import clean_known_hosts as clean_known_hosts_func
from tasks.e2e_framework.tool import get_host, notify, show_connection_message

scenario_name = "aws/dell-powerflex"

# Single host role. get_host looks up dd-Host-<name>; the aws namer prefixes
# "aws-", so dd-Host-aws-powerflex is the exported key and "aws-powerflex" is
# the role string for exec/ssh.
HOST_ROLE = "aws-powerflex"

# SSH hardening: lab hosts are ephemeral; skip host-key prompts and stale
# known_hosts collisions across re-creates.
_SSH_OPTS = "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=15"

# dell_powerflex check name (released integration).
_CHECK_NAME = "dell_powerflex"


@task(
    help={
        "config_path": doc.config_path,
        "install_agent": doc.install_agent,
        "pipeline_id": doc.pipeline_id,
        "agent_version": doc.agent_version,
        "stack_name": doc.stack_name,
        "debug": doc.debug,
        "interactive": doc.interactive,
        "add_known_host": doc.add_known_host,
        "agent_config_path": doc.agent_config_path,
    }
)
def create(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
    pipeline_id: str | None = None,
    install_agent: bool | None = True,
    agent_version: str | None = None,
    debug: bool | None = False,
    interactive: bool | None = True,
    add_known_host: bool | None = True,
    agent_config_path: str | None = None,
) -> None:
    """
    Create the Dell PowerFlex all-in-one lab (single m5.metal host).
    """
    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path,
        key_pair_required=True,
        stack_name=stack_name,
        pipeline_id=pipeline_id,
        install_agent=install_agent,
        agent_version=agent_version,
        debug=debug,
        agent_config_path=agent_config_path,
        needs_agent_containers=False,
    )

    if interactive:
        notify(ctx, "Your Dell PowerFlex lab is now created")

    if add_known_host:
        host = get_host(ctx, HOST_ROLE, scenario_name, stack_name)
        add_known_host_func(ctx, host.address)

    show_connection_message(ctx, HOST_ROLE, full_stack_name, interactive)


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
        "clean_known_hosts": doc.clean_known_hosts,
    }
)
def destroy_lab(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
    clean_known_hosts: bool | None = True,
):
    """
    Destroy the Dell PowerFlex lab.
    """
    host = None
    try:
        host = get_host(ctx, HOST_ROLE, scenario_name, stack_name)
    except Exception:
        host = None

    destroy(
        ctx,
        scenario_name=scenario_name,
        config_path=config_path,
        stack=stack_name,
    )

    if clean_known_hosts and host is not None:
        clean_known_hosts_func(ctx, host.address)


def _ssh_target(ctx: Context, role: str, stack_name: str | None) -> tuple[str, str]:
    if role != HOST_ROLE:
        raise Exit(f"Unknown role '{role}'. This lab has a single host role: '{HOST_ROLE}'.")
    host = get_host(ctx, role, scenario_name, stack_name)
    return host.user, host.address


def _ssh_key_path(ctx: Context, config_path: str | None) -> str | None:
    from tasks.e2e_framework import config

    cfg = config.get_local_config(config_path)
    return cfg.get_aws().privateKeyPath


@task(
    help={
        "role": f"Host role to target (single role: '{HOST_ROLE}')",
        "command": "Command to run on the host",
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def exec(
    ctx: Context,
    command: str,
    role: str = HOST_ROLE,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Run a command on the lab host over SSH.
    """
    user, address = _ssh_target(ctx, role, stack_name)
    key = _ssh_key_path(ctx, config_path)
    key_opt = f"-i {shlex.quote(key)} " if key else ""
    ctx.run(f"ssh {key_opt}{_SSH_OPTS} {user}@{address} {shlex.quote(command)}", pty=True)


@task(
    help={
        "role": f"Host role to target (single role: '{HOST_ROLE}')",
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def ssh(
    ctx: Context,
    role: str = HOST_ROLE,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Open an interactive SSH session to the lab host.
    """
    user, address = _ssh_target(ctx, role, stack_name)
    key = _ssh_key_path(ctx, config_path)
    key_opt = f"-i {shlex.quote(key)} " if key else ""
    ctx.run(f"ssh {key_opt}{_SSH_OPTS} {user}@{address}", pty=True)


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def status(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Run the full `datadog-agent status` on the lab host.
    """
    exec(
        ctx,
        command="sudo datadog-agent status",
        role=HOST_ROLE,
        config_path=config_path,
        stack_name=stack_name,
    )


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def check(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Run the dell_powerflex check once on the lab host.
    """
    exec(
        ctx,
        command=f"sudo -u dd-agent datadog-agent check {_CHECK_NAME}",
        role=HOST_ROLE,
        config_path=config_path,
        stack_name=stack_name,
    )


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    }
)
def reload_check(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Reload the dell_powerflex check config and run it once.
    """
    exec(
        ctx,
        command=f"sudo systemctl restart datadog-agent && sleep 5 && sudo -u dd-agent datadog-agent check {_CHECK_NAME}",
        role=HOST_ROLE,
        config_path=config_path,
        stack_name=stack_name,
    )
