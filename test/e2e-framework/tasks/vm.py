from typing import Optional

import pyperclip
from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task
from pydantic import ValidationError

from tasks.aws import doc as aws_doc
from tasks.config import Config, get_full_profile_path

from . import config, doc, tool


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
        "agent_flavor": doc.agent_flavor,
        "local_package": doc.local_package,
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
    agent_flavor: Optional[str] = None,
    local_package: Optional[str] = None,
) -> None:
    from tasks.aws.vm import create_vm as create_vm_aws

    print('This command is deprecated, please use `aws.create-vm` instead')
    print("Running `aws.create-vm`...")
    create_vm_aws(
        ctx,
        config_path,
        stack_name,
        pipeline_id,
        install_agent,
        install_installer,
        agent_version,
        debug,
        os_family,
        os_version,
        use_fakeintake,
        use_loadBalancer,
        ami_id,
        architecture,
        interactive,
        instance_type,
        no_verify,
        ssh_user,
        agent_flavor=agent_flavor,
        local_package=local_package,
    )


def _filter_aws_resource(resource, instance_id: Optional[str] = None, ip: Optional[str] = None):
    if instance_id and resource["id"] != instance_id:
        return None
    if ip and resource["outputs"]["privateIp"] != ip:
        return None
    return resource


def _get_windows_password(
    ctx: Context,
    cfg: Config,
    full_stack_name: str,
    use_aws_vault: Optional[bool] = True,
    instance_id: Optional[str] = None,
    ip: Optional[str] = None,
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
        aws_account = cfg.get_aws().get_account()
        # TODO: could xref with r['inputs']['keyName']
        key_path = cfg.get_aws().privateKeyPath
        if not key_path:
            raise Exit("No privateKeyPath found in the config.")
        password = tool.get_aws_instance_password_data(
            ctx, vm_id, key_path, aws_account=aws_account, use_aws_vault=use_aws_vault
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
    stack_name: Optional[str] = None,
    config_path: Optional[str] = None,
    ip: Optional[str] = None,
    instance_id: Optional[str] = None,
    use_aws_vault: Optional[bool] = True,
):
    """
    Get the password of a new virtual machine in a stack.
    """
    try:
        cfg = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {get_full_profile_path(config_path)}:{e}")

    if not stack_name:
        raise Exit("Please provide a stack name to connect to.")

    out = _get_windows_password(ctx, cfg, stack_name, use_aws_vault=use_aws_vault, instance_id=instance_id, ip=ip)
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
    stack_name: Optional[str] = None,
    config_path: Optional[str] = None,
    ip: Optional[str] = None,
    instance_id: Optional[str] = None,
    use_aws_vault: Optional[bool] = True,
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
        if "address" not in vm:
            continue
        vm_ip = vm["address"]
        password = vm["password"]
        tool.rdp(ctx, vm_ip)
        print(f"Password for VM {vm_id} ({vm_ip}): {password}")
        print("Username is Administrator, password has been copied to clipboard")
        pyperclip.copy(password)


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
    from tasks.aws.vm import destroy_vm as destroy_vm_aws

    print('This command is deprecated, please use `aws.destroy-vm` instead')
    print("Running `aws.destroy-vm`...")
    destroy_vm_aws(ctx, config_path, stack_name, clean_known_hosts)
