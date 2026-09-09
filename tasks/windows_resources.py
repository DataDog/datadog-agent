import os
import sys

from invoke import task
from invoke.context import Context

from tasks.libs.build.bazel import bazel
from tasks.libs.releasing.version import get_version_numeric_only

MESSAGESTRINGS_MC_PATH = "pkg/util/winutil/messagestrings/messagestrings.mc"
WINDMC_TARGET = "@winlibs_mingw64//:windmc"

_mingw_bin_dir = None


def mingw_bin_dir(ctx: Context) -> str:
    """Absolute path of the bin/ directory of the Bazel-managed MinGW toolchain.

    cquery both fetches @winlibs_mingw64 and resolves the tool path, so nothing
    here depends on Bazel's (unstable) canonical repository naming.
    """
    global _mingw_bin_dir
    if _mingw_bin_dir is None:
        windmc = bazel("cquery", "--output=files", WINDMC_TARGET, capture_output=True).strip()
        execroot = bazel("info", "execution_root", capture_output=True).strip()
        _mingw_bin_dir = os.path.dirname(os.path.join(execroot, windmc))
    return _mingw_bin_dir


def _toolchain_env(ctx: Context, host_target: str) -> dict[str, str]:
    """PATH override making the Bazel-managed windmc/windres reachable.

    Only on a Windows host: the WinLibs distribution is Windows-only, so a Linux
    cross-compile still needs a host binutils-mingw-w64 installation. windres
    shells out to gcc, hence prepending to PATH rather than calling by full path.
    """
    if host_target or sys.platform != 'win32':
        return {}
    return {"PATH": mingw_bin_dir(ctx) + os.pathsep + os.environ["PATH"]}


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
    ctx.run(command, env=_toolchain_env(ctx, host_target))

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

    ctx.run(command, env=_toolchain_env(ctx, host_target))


def versioninfo_vars(ctx):
    ver = get_version_numeric_only(ctx)
    build_maj, build_min, build_patch = ver.split(".")

    return {
        'PY3_RUNTIME': 1,
        'MAJ_VER': build_maj,
        'MIN_VER': build_min,
        'PATCH_VER': build_patch,
        'BUILD_ARCH_x64': 1,
    }
