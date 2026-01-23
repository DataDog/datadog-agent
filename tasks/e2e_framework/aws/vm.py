import json

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.e2e_framework import doc, tool
from tasks.e2e_framework.aws import doc as aws_doc
from tasks.e2e_framework.aws.common import (
    get_architectures,
    get_default_architecture,
    get_default_os_family,
    get_deploy_job,
    get_image_description,
    get_os_families,
)
from tasks.e2e_framework.aws.deploy import deploy
from tasks.e2e_framework.destroy import destroy
from tasks.e2e_framework.tool import add_known_host as add_known_host_func
from tasks.e2e_framework.tool import clean_known_hosts as clean_known_hosts_func
from tasks.e2e_framework.tool import get_host, notify, show_connection_message

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
    config_path: str | None = None,
    stack_name: str | None = None,
    pipeline_id: str | None = None,
    install_agent: bool | None = True,
    install_installer: bool | None = False,
    agent_version: str | None = None,
    debug: bool | None = False,
    os_family: str | None = None,
    os_version: str | None = None,
    use_fakeintake: bool | None = False,
    use_loadBalancer: bool | None = False,
    ami_id: str | None = None,
    architecture: str | None = None,
    interactive: bool | None = True,
    instance_type: str | None = None,
    no_verify: bool | None = False,
    ssh_user: str | None = None,
    add_known_host: bool | None = True,
    agent_flavor: str | None = None,
    agent_config_path: str | None = None,
    local_package: str | None = None,
    latest_ami: bool | None = False,
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
    config_path: str | None = None,
    stack_name: str | None = None,
    clean_known_hosts: bool | None = True,
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
    stack_name: str | None = None,
):
    """
    Show connection details of an aws host.
    """
    host = get_host(ctx, remote_hostname, scenario_name, stack_name)
    print(json.dumps(host.__dict__, indent=4))


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


def _get_os_information(
    ctx: Context, os_family: str | None, arch: str | None, ami_id: str | None
) -> tuple[str, str | None]:
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
                raise Exit("We failed to guess the family of your AMI ID. Please provide it with option -o") from None
        architecture = image["Architecture"]
        if arch is not None and architecture != arch:
            raise Exit(f"The provided architecture is {arch} but the image is {architecture}.")
    else:
        family = _get_os_family(os_family)
        architecture = _get_architecture(arch)
    return family, architecture


def _filter_aws_resource(resource, instance_id: str | None = None, ip: str | None = None):
    if instance_id and resource["id"] != instance_id:
        return None
    if ip and resource["outputs"]["privateIp"] != ip:
        return None
    return resource


def _get_windows_password(
    ctx: Context,
    aws_account: str,
    private_key_path: str,
    full_stack_name: str,
    use_aws_vault: bool | None = True,
    instance_id: str | None = None,
    ip: str | None = None,
):
    resources = tool.get_stack_json_resources(ctx, full_stack_name)
    vms = []
    for r in resources:
        if not r["type"].startswith("aws:ec2/instance:Instance"):
            continue
        vms.append(r)
    if not vms:
        raise Exit("No VM found in the stack.")

    out = []
    for r in vms:
        if not _filter_aws_resource(r, instance_id, ip):
            continue
        vm_id = r["id"]

        # TODO: could xref with r['inputs']['keyName']
        if not private_key_path:
            raise Exit("No privateKeyPath found in the config.")
        password = tool.get_aws_instance_password_data(
            ctx, vm_id, private_key_path, aws_account=aws_account, use_aws_vault=use_aws_vault
        )
        if password:
            out.append({"vm_id": vm_id, "resource": r, "password": password})
    return out


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": "Name of stack that contains hosts to RDP into",
        "use_aws_vault": doc.use_aws_vault,
        "ip": "Filter to VM with this IP address",
        "instance_id": "Filter to VM with this id",
    }
)
def get_vm_password(
    ctx: Context,
    stack_name: str | None = None,
    config_path: str | None = None,
    ip: str | None = None,
    instance_id: str | None = None,
    use_aws_vault: bool | None = True,
):
    """
    Get the password of a new virtual machine in a stack.
    """
    from pydantic import ValidationError

    from tasks.e2e_framework import config

    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}:{e}") from e

    if not stack_name:
        raise Exit("Please provide a stack name to connect to.")

    out = _get_windows_password(
        ctx,
        cfg.get_aws().get_account(),
        cfg.get_aws().privateKeyPath,
        stack_name,
        use_aws_vault=use_aws_vault,
        instance_id=instance_id,
        ip=ip,
    )
    if not out:
        raise Exit(
            "No VM found in the stack, or no password available. Verify that keyPairName and publicKeyPath are an RSA key. run `inv setup.debug` for automated help."
        )
    for vm in out:
        vm_id = vm["vm_id"]
        vm_ip = vm["resource"]["outputs"]["privateIp"]
        password = vm["password"]
        print(f"Password for VM {vm_id} ({vm_ip}): {password}")


@task(
    help={
        "config_path": doc.config_path,
        "stack_name": "Name of stack that contains hosts to RDP into",
        "use_aws_vault": doc.use_aws_vault,
        "ip": "Filter to VM with this IP address",
        "instance_id": "Filter to VM with this id",
    }
)
def rdp_vm(
    ctx: Context,
    stack_name: str | None = None,
    config_path: str | None = None,
    ip: str | None = None,
    instance_id: str | None = None,
    use_aws_vault: bool | None = True,
):
    """
    Open an RDP connection to a new virtual machine in a stack.
    """

    if not stack_name:
        raise Exit("Please provide a stack name to connect to.")

    out = tool.get_stack_json_outputs(ctx, stack_name)
    if not out:
        raise Exit("No VM found in the stack.")

    for vm_id, vm in out.items():
        import pyperclip

        if "address" not in vm:
            continue
        vm_ip = vm["address"]
        password = vm["password"]
        tool.rdp(ctx, vm_ip)
        print(f"Password for VM {vm_id} ({vm_ip}): {password}")
        print("Username is Administrator, password has been copied to clipboard")

        pyperclip.copy(password)
