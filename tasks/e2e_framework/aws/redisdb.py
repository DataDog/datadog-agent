# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2025-present Datadog, Inc.

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import config, doc, tool
from tasks.e2e_framework.aws import doc as aws_doc
from tasks.e2e_framework.aws.common import get_architectures, get_default_architecture
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

scenario_name = "aws/redisdb"

# The RemoteHost output key emitted by the scenario's Pulumi program.
# Matches the name passed to ec2.NewVM (params.Name defaults to "redisdb").
_REMOTE_HOST_NAME = "aws-redisdb"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _get_architecture(architecture: str | None) -> str:
    """Validate *architecture* against the supported list and return it."""
    architectures = get_architectures()
    if architecture is None:
        architecture = get_default_architecture()
    if architecture.lower() not in architectures:
        raise Exit(
            f"The architecture '{architecture}' is not supported. " f"Possible values are {', '.join(architectures)}"
        )
    return architecture


def _build_ssh_command(
    ctx: Context,
    full_stack_name: str,
    config_path: str | None,
) -> str:
    """
    Return the SSH command string for the Docker host of the redisdb stack.

    If a private key path is configured it is injected via ``-i``.
    """
    from pydantic import ValidationError

    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    remote_host = tool.RemoteHost(_REMOTE_HOST_NAME, outputs)
    user = remote_host.user
    address = remote_host.address

    # Build base command
    ssh_cmd = "ssh"

    # Inject private key when available in config
    try:
        cfg = config.get_local_config(config_path)
        private_key_path = cfg.get_aws().privateKeyPath
        if private_key_path:
            ssh_cmd += f" -i {private_key_path}"
    except (ValidationError, Exception):
        # Best-effort: proceed without -i if config is absent or malformed
        pass

    if remote_host.port:
        ssh_cmd += f" -p {remote_host.port}"

    ssh_cmd += f" {user}@{address}"
    return ssh_cmd


# ---------------------------------------------------------------------------
# invoke tasks
# ---------------------------------------------------------------------------


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
        "install_agent": doc.install_agent,
        "agent_version": doc.container_agent_version,
        "architecture": aws_doc.architecture,
        "use_fakeintake": doc.fakeintake,
        "use_loadBalancer": doc.use_loadBalancer,
        "interactive": doc.interactive,
        "full_image_path": doc.full_image_path,
        "agent_flavor": doc.agent_flavor,
        "agent_env": doc.agent_env,
    },
)
def create_redisdb(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
    install_agent: bool | None = True,
    agent_version: str | None = None,
    architecture: str | None = None,
    use_fakeintake: bool | None = False,
    use_loadBalancer: bool | None = False,
    interactive: bool | None = True,
    full_image_path: str | None = None,
    agent_flavor: str | None = None,
    agent_env: str | None = None,
):
    """
    Create a Docker-on-EC2 environment with a Redis database workload and an
    optional containerised Datadog Agent (aws/redisdb scenario).

    The EC2 host runs Docker.  A Redis container is started alongside the
    Agent so that the Agent's Redis integration can collect metrics
    immediately after provisioning.

    Use ``aws.destroy-redisdb`` to tear the environment down and
    ``aws.connect-redisdb`` to open an SSH session on the host.
    """
    extra_flags = {
        "ddinfra:osDescriptor": f"::{_get_architecture(architecture)}",
        "ddinfra:deployFakeintakeWithLoadBalancer": use_loadBalancer,
    }

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path,
        key_pair_required=True,
        stack_name=stack_name,
        install_agent=install_agent,
        agent_version=agent_version,
        use_fakeintake=use_fakeintake,
        extra_flags=extra_flags,
        full_image_path=full_image_path,
        agent_flavor=agent_flavor,
        agent_env=agent_env,
    )

    if interactive:
        tool.notify(ctx, "Your redisdb environment is now created")

    _show_connection_message(ctx, full_stack_name, config_path, interactive)


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    },
)
def destroy_redisdb(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Destroy an environment previously created by ``invoke aws.create-redisdb``.
    """
    destroy(
        ctx,
        scenario_name=scenario_name,
        config_path=config_path,
        stack=stack_name,
    )


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
    },
)
def connect_redisdb(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
):
    """
    Open an SSH session on the Docker host of a redisdb environment.

    The SSH command is constructed from the Pulumi stack outputs and the
    private key path stored in the local E2E config (when present).
    Run ``invoke aws.create-redisdb`` first to provision the environment.
    """
    full_stack_name = tool.get_stack_name(stack_name, scenario_name)
    ssh_cmd = _build_ssh_command(ctx, full_stack_name, config_path)

    print(f"\nConnecting to redisdb host:\n  {ssh_cmd}\n")
    # Use pty so the SSH session is fully interactive (stdin/stdout attached).
    ctx.run(ssh_cmd, pty=True)


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _show_connection_message(
    ctx: Context,
    full_stack_name: str,
    config_path: str | None,
    copy_to_clipboard: bool | None,
) -> None:
    """Print the SSH command that connects to the Docker host and optionally
    copy it to the system clipboard."""
    ssh_cmd = _build_ssh_command(ctx, full_stack_name, config_path)

    print(
        f"\nYour redisdb environment is ready.\n"
        f"Run the following command to connect to the host:\n\n"
        f"  {ssh_cmd}\n"
    )

    if copy_to_clipboard:
        import pyperclip

        input("Press a key to copy the SSH command to the clipboard...")
        pyperclip.copy(ssh_cmd)
