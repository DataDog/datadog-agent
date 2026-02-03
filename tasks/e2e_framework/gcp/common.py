def get_default_os_family() -> str:
    return "ubuntu"


def get_os_families() -> list[str]:
    return [
        get_default_os_family(),
    ]


def get_package_for_os(os: str) -> str:
    package_map = {
        get_default_os_family(): "deb",
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
        assert arch == 'x64', f'Invalid architecture {arch} for Windows'
    elif os == 'suse':
        suffix = f'_{arch}-{v}'
    elif pkg in ('deb', 'rpm'):
        suffix = f'-{v}_{arch}'
    else:
        raise RuntimeError(f'Cannot deduce deploy job from {os}::{arch}')

    return f'deploy_{pkg}_testing{suffix}'


def get_architectures() -> list[str]:
    return [get_default_architecture(), "arm64"]


def get_default_architecture() -> str:
    return "x86_64"
