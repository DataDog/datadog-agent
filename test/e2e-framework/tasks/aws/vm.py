import json
from typing import Optional, Tuple

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks import doc
from tasks.aws import doc as aws_doc
from tasks.aws.common import (
    get_architectures,
    get_default_architecture,
    get_default_os_family,
    get_deploy_job,
    get_image_description,
    get_os_families,
)
from tasks.aws.deploy import deploy
from tasks.destroy import destroy
from tasks.tool import add_known_host as add_known_host_func
from tasks.tool import clean_known_hosts as clean_known_hosts_func
from tasks.tool import get_host, notify, show_connection_message

default_public_path_key_name = "ddinfra:aws/defaultPublicKeyPath"
scenario_name = "aws/vm"
remote_hostname = "aws-vm"


@task(
    help={
        "config_path": doc.config_path,
        "install_agent": doc.install_agent,
        "install_installer": doc.install_installer,
        "pipeline_id": doc.pipeline_id,
        "agent_version": doc.agent_version,
        "stack_name": doc.stack_name,
        "debug": doc.debug,
        "os_family": aws_doc.os_family,
        "use_fakeintake": doc.fakeintake,
        "use_loadBalancer": doc.use_loadBalancer,
        "ami_id": aws_doc.ami_id,
        "architecture": aws_doc.architecture,
        "interactive": doc.interactive,
        "instance_type": aws_doc.instance_type,
        "no_verify": doc.no_verify,
        "ssh_user": doc.ssh_user,
        "os_version": doc.os_version,
        "add_known_host": doc.add_known_host,
        "agent_flavor": doc.agent_flavor,
        "agent_config_path": doc.agent_config_path,
        "local_package": doc.local_package,
        "latest_ami": doc.latest_ami,
    }
)
def create_vm(
    ctx: Context,
    config_path: Optional[str] = None,
    stack_name: Optional[str] = None,
    pipeline_id: Optional[str] = None,
    install_agent: Optional[bool] = True,
    install_installer: Optional[bool] = False,
    agent_version: Optional[str] = None,
    debug: Optional[bool] = False,
    os_family: Optional[str] = None,
    os_version: Optional[str] = None,
    use_fakeintake: Optional[bool] = False,
    use_loadBalancer: Optional[bool] = False,
    ami_id: Optional[str] = None,
    architecture: Optional[str] = None,
    interactive: Optional[bool] = True,
    instance_type: Optional[str] = None,
    no_verify: Optional[bool] = False,
    ssh_user: Optional[str] = None,
    add_known_host: Optional[bool] = True,
    agent_flavor: Optional[str] = None,
    agent_config_path: Optional[str] = None,
    local_package: Optional[str] = None,
    latest_ami: Optional[bool] = False,
) -> None:
    """
    Create a new virtual machine on aws.
    """

    extra_flags = {}
    if os_family == "macos":
        extra_flags["ddinfra:aws/useMacosCompatibleSubnets"] = True
    os_family, os_arch = _get_os_information(ctx, os_family, architecture, ami_id)
    deploy_job = None if no_verify or not pipeline_id else get_deploy_job(os_family, os_arch, agent_version)
    extra_flags["ddinfra:osDescriptor"] = f"{os_family}:{os_version if os_version else ''}:{os_arch}"
    extra_flags["ddinfra:deployFakeintakeWithLoadBalancer"] = use_loadBalancer

    if ami_id is not None:
        extra_flags["ddinfra:osImageID"] = ami_id
    if latest_ami is not None:
        extra_flags["ddinfra:osImageIDUseLatest"] = latest_ami

    if use_fakeintake and not install_agent:
        print(
            "[WARNING] It is currently not possible to deploy a VM with fakeintake and without agent. Your VM will start without fakeintake."
        )
    if instance_type:
        if not architecture or architecture.lower() == get_default_architecture():
            extra_flags["ddinfra:aws/defaultInstanceType"] = instance_type
        else:
            extra_flags["ddinfra:aws/defaultARMInstanceType"] = instance_type

    if ssh_user:
        extra_flags["ddinfra:sshUser"] = ssh_user

    full_stack_name = deploy(
        ctx,
        scenario_name,
        config_path,
        key_pair_required=True,
        public_key_required=(os_family.lower() == "windows"),
        stack_name=stack_name,
        pipeline_id=pipeline_id,
        install_agent=install_agent,
        install_installer=install_installer,
        agent_version=agent_version,
        debug=debug,
        extra_flags=extra_flags,
        use_fakeintake=use_fakeintake,
        deploy_job=deploy_job,
        agent_flavor=agent_flavor,
        agent_config_path=agent_config_path,
        local_package=local_package,
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


@task(
    help={
        "stack_name": doc.stack_name,
    }
)
def show_vm(
    ctx: Context,
    stack_name: Optional[str] = None,
):
    """
    Show connection details of an aws host.
    """
    host = get_host(ctx, remote_hostname, scenario_name, stack_name)
    print(json.dumps(host.__dict__, indent=4))


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


def _get_os_information(
    ctx: Context, os_family: Optional[str], arch: Optional[str], ami_id: Optional[str]
) -> Tuple[str, Optional[str]]:
    family, architecture = os_family, None
    if ami_id is not None:
        image = get_image_description(ctx, ami_id)
        if family is None:  # Try to guess the distribution
            os_families = get_os_families()
            try:
                if "Description" in image:
                    image_info = image["Description"]
                else:
                    image_info = image["Name"]
                image_info = image_info.lower().replace(" ", "")
                family = next(os for os in os_families if os in image_info)

            except StopIteration:
                raise Exit("We failed to guess the family of your AMI ID. Please provide it with option -o")
        architecture = image["Architecture"]
        if arch is not None and architecture != arch:
            raise Exit(f"The provided architecture is {arch} but the image is {architecture}.")
    else:
        family = _get_os_family(os_family)
        architecture = _get_architecture(arch)
    return family, architecture
