import os
import sys

from invoke import task

from tasks.libs.releasing.version import get_version_numeric_only

MESSAGESTRINGS_MC_PATH = "pkg/util/winutil/messagestrings/messagestrings.mc"


@task
def build_messagetable(
    ctx,
    target='pe-x86-64',
    host_target='',  # prefix of the toolchain used to cross-compile, for instance x86_64-w64-mingw32
):
    """
    Build the header and resource for the MESSAGETABLE shared between agent binaries.
    """
    messagefile = MESSAGESTRINGS_MC_PATH

    root = os.path.dirname(messagefile)

    # Generate the message header and resource file
    windmc = "windmc"
    if not host_target and sys.platform.startswith('linux'):
        host_target = "x86_64-w64-mingw32"

    if host_target:
        windmc = host_target + "-" + windmc

    command = f"{windmc} --target {target} -r {root} -h {root} {messagefile}"
    ctx.run(command)

    build_rc(ctx, f'{root}/messagestrings.rc', target=target, host_target=host_target)


def build_rc(ctx, rc_file, vars=None, out=None, target='pe-x86-64', host_target=''):
    if vars is None:
        vars = {}

    if out is None:
        root = os.path.dirname(rc_file)
        out = f'{root}/rsrc.syso'

    # Build the binary resource
    # go automatically detects+includes .syso files
    windres = "windres"
    if not host_target and sys.platform.startswith('linux'):
        host_target = "x86_64-w64-mingw32"

    if host_target:
        windres = host_target + "-" + windres

    command = f"{windres} --target {target} -i {rc_file} -O coff -o {out}"
    for key, value in vars.items():
        command += f" --define {key}={value}"

    ctx.run(command)


def versioninfo_vars(ctx, major_version='7'):
    ver = get_version_numeric_only(ctx, major_version=major_version)
    build_maj, build_min, build_patch = ver.split(".")

    return {
        'PY3_RUNTIME': 1,
        'MAJ_VER': build_maj,
        'MIN_VER': build_min,
        'PATCH_VER': build_patch,
        'BUILD_ARCH_x64': 1,
    }
