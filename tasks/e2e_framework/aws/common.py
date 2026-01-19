import json
from io import StringIO
from typing import Any

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
    return f"aws-vault exec sso-{aws_account}-account-admin -- "


def get_image_description(ctx: Context, ami_id: str) -> Any:
    buffer = StringIO()
    ctx.run(
        f"aws-vault exec sso-agent-sandbox-account-admin -- aws ec2 describe-images --image-ids {ami_id}",
        out_stream=buffer,
    )
    result = json.loads(buffer.getvalue())
    if len(result["Images"]) > 1:
        raise Exit(f"The AMI id {ami_id} returns more than one definition.")
    else:
        return result["Images"][0]
