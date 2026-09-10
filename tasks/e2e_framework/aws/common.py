import json
from io import StringIO
from typing import Any

import yaml
from invoke.context import Context
from invoke.exceptions import Exit


def get_default_os_family() -> str:
    return "ubuntu"


def get_os_families() -> list[str]:
    return [
        get_default_os_family(),
        "windows",
        "amazonlinux",
        "amazonlinuxdocker",
        "debian",
        "redhat",
        "suse",
        "fedora",
        "centos",
        "rockylinux",
        "macos",
    ]


def get_package_for_os(os: str) -> str:
    package_map = {
        get_default_os_family(): "deb",
        "windows": "windows",
        "amazonlinux": "rpm",
        "amazonlinuxdocker": "rpm",
        "debian": "deb",
        "redhat": "rpm",
        "suse": "suse_rpm",
        "fedora": "rpm",
        "centos": "rpm",
        "rockylinux": "rpm",
        "macos": "dmg",
    }

    return package_map[os]


def get_deploy_job(os: str, arch: str | None, agent_version: str | None = None) -> str:
    """
    Returns the deploy job name within the datadog agent repo that creates
    images used in create-vm
    """
    pkg = get_package_for_os(os)
    if agent_version is None:
        v = 'a7'
    else:
        major = agent_version.split('.')[0]
        assert major in ('6', '7'), f'Invalid agent version {agent_version}'
        v = f'a{major}'

    if arch == 'x86_64':
        arch = 'x64'

    # Construct job name
    if os == 'windows':
        suffix = f'-{v}'
        assert arch == 'x64', f'Invalid architecure {arch} for Windows'
    elif os == 'suse':
        suffix = f'_{arch}-{v}'
    elif pkg in ('deb', 'rpm', 'dmg'):
        suffix = f'-{v}_{arch}'
    else:
        raise RuntimeError(f'Cannot deduce deploy job from {os}::{arch}')

    return f'deploy_{pkg}_testing{suffix}'


def get_architectures() -> list[str]:
    return [get_default_architecture(), "arm64"]


def get_default_architecture() -> str:
    return "x86_64"


def get_aws_wrapper(
    aws_account: str,
) -> str:
    return f"aws-vault exec sso-{aws_account}-account-admin-8h -- "


def show_eks_connection_message(
    ctx: Context, full_stack_name: str, config_path: str | None, interactive: bool | None = True
):
    """
    Write the stack's kubeconfig as {full_stack_name}-kubeconfig.yaml in the current
    working directory, then print (and optionally copy) the kubectl command to reach the
    cluster. Reads the dd-Cluster-eks stack output, so it serves the scenarios built on
    scenarios/aws/eks.
    """
    from pydantic import ValidationError

    from tasks.e2e_framework import config, tool

    outputs = tool.get_stack_json_outputs(ctx, full_stack_name)
    kubeconfig_output = json.loads(outputs["dd-Cluster-eks"]["kubeConfig"])
    kubeconfig = f"{full_stack_name}-kubeconfig.yaml"
    tool.write_secret_file(kubeconfig, yaml.dump(kubeconfig_output))

    try:
        local_config = config.get_local_config(config_path)
    except ValidationError as e:
        raise Exit(f"Error in config {config.get_full_profile_path(config_path)}:{e}") from e

    command = f"KUBECONFIG={kubeconfig} {get_aws_wrapper(local_config.get_aws().get_account())} kubectl get nodes"

    print(f"\nYou can run the following command to connect to the EKS cluster\n\n{command}\n")

    if interactive:
        tool.copy_to_clipboard_if_supported(command, prompt="Press a key to copy command to clipboard...")


def get_image_description(ctx: Context, ami_id: str) -> Any:
    buffer = StringIO()
    ctx.run(
        f"aws-vault exec sso-agent-sandbox-account-admin-8h -- aws ec2 describe-images --image-ids {ami_id}",
        out_stream=buffer,
    )
    result = json.loads(buffer.getvalue())
    if len(result["Images"]) > 1:
        raise Exit(f"The AMI id {ami_id} returns more than one definition.")
    else:
        return result["Images"][0]
