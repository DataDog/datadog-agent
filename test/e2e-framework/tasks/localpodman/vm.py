from typing import Optional

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task
from pydantic_core._pydantic_core import ValidationError

from tasks import config, doc
from tasks.deploy import deploy
from tasks.destroy import destroy
from tasks.tool import add_known_host as add_known_host_func
from tasks.tool import clean_known_hosts as clean_known_hosts_func
from tasks.tool import get_host, notify, show_connection_message

scenario_name = "localpodman/vm"
remote_hostname = "local-podman-vm"


@task(
    help={
        "config_path": doc.config_path,
        "install_agent": doc.install_agent,
        "pipeline_id": doc.pipeline_id,
        "agent_version": doc.agent_version,
        "stack_name": doc.stack_name,
        "debug": doc.debug,
        "use_fakeintake": doc.fakeintake,
        "interactive": doc.interactive,
        "add_known_host": doc.add_known_host,
        "agent_flavor": doc.agent_flavor,
        "agent_config_path": doc.agent_config_path,
    }
)
def create_vm(
    ctx: Context,
    config_path: Optional[str] = None,
    stack_name: Optional[str] = None,
    pipeline_id: Optional[str] = None,
    install_agent: Optional[bool] = True,
    agent_version: Optional[str] = None,
    debug: Optional[bool] = False,
    use_fakeintake: Optional[bool] = False,
    interactive: Optional[bool] = True,
    add_known_host: Optional[bool] = True,
    agent_flavor: Optional[str] = None,
    agent_config_path: Optional[str] = None,
) -> None:
    """
    Create a new virtual machine on local podman.
    """

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}") from e

    if not cfg.get_local().publicKeyPath:
        raise Exit("The field `local.publicKeyPath` is required in the config file")

    extra_flags = {
        "ddinfra:local/defaultPublicKeyPath": cfg.get_local().publicKeyPath,
    }

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path,
        stack_name=stack_name,
        pipeline_id=pipeline_id,
        install_agent=install_agent,
        agent_version=agent_version,
        debug=debug,
        extra_flags=extra_flags,
        use_fakeintake=use_fakeintake,
        agent_flavor=agent_flavor,
        agent_config_path=agent_config_path,
    )

    if interactive:
        notify(ctx, "Your VM is now created")

    if add_known_host:
        host = get_host(ctx, remote_hostname, scenario_name, stack_name)
        add_known_host_func(ctx, host.address)

    show_connection_message(ctx, remote_hostname, full_stack_name, interactive)


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": doc.stack_name,
        "clean_known_hosts": doc.clean_known_hosts,
    }
)
def destroy_vm(
    ctx: Context,
    config_path: Optional[str] = None,
    stack_name: Optional[str] = None,
    clean_known_hosts: Optional[bool] = True,
):
    """
    Destroy a new virtual machine on aws.
    """

    host = get_host(ctx, remote_hostname, scenario_name, stack_name)

    destroy(
        ctx,
        scenario_name=scenario_name,
        config_path=config_path,
        stack=stack_name,
    )

    if clean_known_hosts:
        clean_known_hosts_func(ctx, host.address)
