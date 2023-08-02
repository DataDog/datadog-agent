import os

from .utils import get_version_numeric_only, get_win_py_runtime_var


def arch_to_windres_target(
    arch='x64',
):
    if arch == 'x86':
        return 'pe-ie86'
    elif arch == 'x64':
        return 'pe-x86-64'
    else:
        raise Exception(f"Unsupported architecture: {arch}")


def build_messagetable(
    ctx,
    env,
    arch='x64',
):
    windres_target = arch_to_windres_target(arch)

    root = "pkg/util/winutil/messagestrings"

    messagefile = f'{root}/messagestrings.mc'

    # Generate the message header and resource file
    command = f"windmc --target {windres_target} -r {root} -h {root} {messagefile}"
    ctx.run(command, env=env)

    build_rc(ctx, env, f'{root}/messagestrings.rc', arch=arch)


def build_rc(ctx, env, rc_file, arch='x64', vars=None, out=None):
    if vars is None:
        vars = {}

    windres_target = arch_to_windres_target(arch)

    if out is None:
        root = os.path.dirname(rc_file)
        out = f'{root}/rsrc.syso'

    # Build the binary resource
    # go automatically detects+includes .syso files
    command = f"windres --target {windres_target} -i {rc_file} -O coff -o {out}"
    for key, value in vars.items():
        command += f" --define {key}={value}"

    ctx.run(command, env=env)


def versioninfo_vars(
    ctx,
    major_version='7',
    python_runtimes='3',
    arch='x64',
):
    py_runtime_var = get_win_py_runtime_var(python_runtimes)
    ver = get_version_numeric_only(ctx, major_version=major_version)
    build_maj, build_min, build_patch = ver.split(".")

    return {
        f'{py_runtime_var}': 1,
        'MAJ_VER': build_maj,
        'MIN_VER': build_min,
        'PATCH_VER': build_patch,
        f'BUILD_ARCH_{arch}': 1,
    }
