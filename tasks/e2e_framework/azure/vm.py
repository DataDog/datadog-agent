from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.azure import doc as azure_doc
from tasks.e2e_framework.azure.common import (
    get_architectures,
    get_default_architecture,
    get_default_os_family,
    get_deploy_job,
    get_os_families,
)
from tasks.e2e_framework.deploy import deploy
from tasks.e2e_framework.destroy import destroy
from tasks.e2e_framework.tool import add_known_host as add_known_host_func
from tasks.e2e_framework.tool import clean_known_hosts as clean_known_hosts_func
from tasks.e2e_framework.tool import get_host, show_connection_message

scenario_name = "az/vm"
remote_hostname = "az-vm"


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
        "os_family": azure_doc.os_family,
        "architecture": azure_doc.architecture,
        "instance_type": azure_doc.instance_type,
        "os_version": doc.os_version,
        "add_known_host": doc.add_known_host,
        "agent_flavor": doc.agent_flavor,
        "agent_config_path": doc.agent_config_path,
    }
)
def create_vm(
    ctx: Context,
    config_path: str | None = None,
    stack_name: str | None = None,
    install_agent: bool | None = True,
    install_installer: bool | None = False,
    agent_version: str | None = None,
    debug: bool | None = False,
    interactive: bool | None = True,
    ssh_user: str | None = None,
    account: str | None = None,
    os_family: str | None = None,
    os_version: str | None = None,
    architecture: str | None = None,
    instance_type: str | None = None,
    deploy_job: str | None = None,
    no_verify: bool | None = False,
    use_fakeintake: bool | None = False,
    add_known_host: bool | None = True,
    agent_flavor: str | None = None,
    agent_config_path: str | None = None,
) -> None:
    """
    Create a new virtual machine on azure.
    """

    from pydantic_core._pydantic_core import ValidationError

    from tasks.e2e_framework import config

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}") from e

    if not cfg.get_azure().publicKeyPath:
        raise Exit("The field `azure.publicKeyPath` is required in the config file")

    os_family, os_arch = _get_os_information(os_family, architecture)
    _deploy_job = None if no_verify else get_deploy_job(os_family, os_arch, agent_version)

    extra_flags = {
        "ddinfra:env": f"az/{account if account else cfg.get_azure().account}",
        "ddinfra:az/defaultPublicKeyPath": cfg.get_azure().publicKeyPath,
        "ddinfra:osDescriptor": f"{os_family}:{os_version if os_version else ''}:{os_arch}",
    }

    if instance_type:
        if architecture is None or architecture.lower() == get_default_architecture():
            extra_flags["ddinfra:az/defaultInstanceType"] = instance_type
        else:
            extra_flags["ddinfra:az/defaultARMInstanceType"] = instance_type

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
    config_path: str | None = None,
    stack_name: str | None = None,
    clean_known_hosts: bool | None = True,
):
    """
    Destroy a new virtual machine on azure.
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


def _get_os_information(os_family: str | None, arch: str | None) -> tuple[str, str | None]:
    return _get_os_family(os_family), _get_architecture(arch)


def _get_os_family(os_family: str | None) -> str:
    os_families = get_os_families()
    if not os_family:
        os_family = get_default_os_family()
    if os_family.lower() not in os_families:
        raise Exit(f"The os family '{os_family}' is not supported. Possibles values are {', '.join(os_families)}")
    return os_family


def _get_architecture(architecture: str | None) -> str:
    architectures = get_architectures()
    if not architecture:
        architecture = get_default_architecture()
    if architecture.lower() not in architectures:
        raise Exit(f"The os family '{architecture}' is not supported. Possibles values are {', '.join(architectures)}")
    return architecture
