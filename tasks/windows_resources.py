import os

from invoke import task

from tasks.libs.releasing.version import get_version_numeric_only

MESSAGESTRINGS_MC_PATH = "pkg/util/winutil/messagestrings/messagestrings.mc"


@task
def build_messagetable(
    ctx,
):
    """
    Build the header and resource for the MESSAGETABLE shared between agent binaries.
    """
    windres_target = 'pe-x86-64'

    messagefile = MESSAGESTRINGS_MC_PATH

    root = os.path.dirname(messagefile)

    # Generate the message header and resource file
    command = f"windmc --target {windres_target} -r {root} -h {root} {messagefile}"
    ctx.run(command)

    build_rc(ctx, f'{root}/messagestrings.rc')


def build_rc(ctx, rc_file, vars=None, out=None):
    if vars is None:
        vars = {}

    windres_target = 'pe-x86-64'

    if out is None:
        root = os.path.dirname(rc_file)
        out = f'{root}/rsrc.syso'

    # Build the binary resource
    # go automatically detects+includes .syso files
    command = f"windres --target {windres_target} -i {rc_file} -O coff -o {out}"
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
