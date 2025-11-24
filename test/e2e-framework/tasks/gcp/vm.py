from typing import Optional, Tuple

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task
from pydantic_core._pydantic_core import ValidationError

from tasks import config, doc, tool
from tasks.config import get_full_profile_path
from tasks.deploy import deploy
from tasks.destroy import destroy
from tasks.gcp import doc as gcp_doc
from tasks.gcp.common import (
    get_architectures,
    get_default_architecture,
    get_default_os_family,
    get_deploy_job,
    get_os_families,
)
from tasks.tool import (
    add_known_host as add_known_hosts_func,
)
from tasks.tool import (
    clean_known_hosts as clean_known_hosts_func,
)
from tasks.tool import (
    get_host,
    show_connection_message,
)

scenario_name = "gcp/vm"
remote_hostname = "gcp-vm"


@task(
    help={
        "config_path": doc.config_path,
        "install_agent": doc.install_agent,
        "install_installer": doc.install_installer,
        "agent_version": doc.agent_version,
        "stack_name": doc.stack_name,
        "debug": doc.debug,
        "interactive": doc.interactive,
        "ssh_user": doc.ssh_user,
        "os_family": gcp_doc.os_family,
        "architecture": gcp_doc.architecture,
        "instance_type": gcp_doc.instance_type,
        "os_version": doc.os_version,
        "add_known_host": doc.add_known_host,
        "agent_flavor": doc.agent_flavor,
        "agent_config_path": doc.agent_config_path,
    }
)
def create_vm(
    ctx: Context,
    config_path: Optional[str] = None,
    stack_name: Optional[str] = None,
    install_agent: Optional[bool] = True,
    install_installer: Optional[bool] = False,
    agent_version: Optional[str] = None,
    debug: Optional[bool] = False,
    interactive: Optional[bool] = True,
    ssh_user: Optional[str] = None,
    account: Optional[str] = None,
    os_family: Optional[str] = None,
    os_version: Optional[str] = None,
    architecture: Optional[str] = None,
    instance_type: Optional[str] = None,
    deploy_job: Optional[str] = None,
    no_verify: Optional[bool] = False,
    use_fakeintake: Optional[bool] = False,
    add_known_host: Optional[bool] = True,
    agent_flavor: Optional[str] = None,
    agent_config_path: Optional[str] = None,
) -> None:
    """
    Create a new virtual machine on gcp.
    """

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {get_full_profile_path(config_path)}") from e

    if not cfg.get_gcp().publicKeyPath:
        raise Exit("The field `gcp.publicKeyPath` is required in the config file")

    os_family, os_arch = _get_os_information(os_family, architecture)
    _deploy_job = None if no_verify else get_deploy_job(os_family, os_arch, agent_version)

    extra_flags = {
        "ddinfra:env": f"gcp/{account if account else cfg.get_gcp().account}",
        "ddinfra:gcp/defaultPublicKeyPath": cfg.get_gcp().publicKeyPath,
        "ddinfra:osDescriptor": f"{os_family}:{os_version if os_version else ''}:{os_arch}",
    }

    if instance_type:
        if architecture is None or architecture.lower() == get_default_architecture():
            extra_flags["ddinfra:gcp/defaultInstanceType"] = instance_type
        else:
            extra_flags["ddinfra:gcp/defaultARMInstanceType"] = instance_type

    if ssh_user:
        extra_flags["ddinfra:sshUser"] = ssh_user

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path,
        stack_name=stack_name,
        install_agent=install_agent,
        install_installer=install_installer,
        agent_version=agent_version,
        debug=debug,
        extra_flags=extra_flags,
        use_fakeintake=use_fakeintake,
        agent_flavor=agent_flavor,
        agent_config_path=agent_config_path,
    )

    if interactive:
        tool.notify(ctx, "Your VM is now created")

    if add_known_host:
        host = get_host(ctx, remote_hostname, scenario_name, stack_name)
        add_known_hosts_func(ctx, host.address)

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
    Destroy a new virtual machine on gcp.
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


def _get_os_information(os_family: Optional[str], arch: Optional[str]) -> Tuple[str, Optional[str]]:
    return _get_os_family(os_family), _get_architecture(arch)


def _get_os_family(os_family: Optional[str]) -> str:
    os_families = get_os_families()
    if not os_family:
        os_family = get_default_os_family()
    if os_family.lower() not in os_families:
        raise Exit(f"The os family '{os_family}' is not supported. Possibles values are {', '.join(os_families)}")
    return os_family


def _get_architecture(architecture: Optional[str]) -> str:
    architectures = get_architectures()
    if not architecture:
        architecture = get_default_architecture()
    if architecture.lower() not in architectures:
        raise Exit(f"The os family '{architecture}' is not supported. Possibles values are {', '.join(architectures)}")
    return architecture
