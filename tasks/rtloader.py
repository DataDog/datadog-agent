"""
RtLoader namespaced tasks
"""

import errno
import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.build.bazel import bazel
from tasks.libs.common.utils import gitlab_section


def get_rtloader_path():
    here = os.path.abspath(os.path.dirname(__file__))
    return os.path.abspath(os.path.join(here, '..', 'rtloader'))


def get_rtloader_build_path():
    return os.path.join(get_rtloader_path(), 'build')


def get_dev_path():
    here = os.path.abspath(os.path.dirname(__file__))
    return os.path.abspath(os.path.join(here, '..', 'dev'))


def run_make_command(ctx, command=""):
    ctx.run(f"make -C {get_rtloader_build_path()} {command}", err_stream=sys.stdout)


@task
def make(ctx, install_prefix=None, cmake_options=''):
    dev_path = get_dev_path()
    prefix = install_prefix or dev_path

    if cmake_options.find("-G") == -1:
        cmake_options += " -G \"Unix Makefiles\""

    cmake_args = cmake_options + f" -DCMAKE_INSTALL_PREFIX:PATH={prefix}"
    if os.getenv('DD_CMAKE_TOOLCHAIN'):
        cmake_args += f' --toolchain {os.getenv("DD_CMAKE_TOOLCHAIN")}'

    rtloader_build_path = get_rtloader_build_path()

    if sys.platform == 'darwin':
        cmake_args += " -DCMAKE_OSX_DEPLOYMENT_TARGET=10.13"
    elif sys.platform == 'linux':
        # Set RPATH so that libdatadog-agent-rtloader.so can find libdatadog-agent-three.so
        # at runtime without needing LD_LIBRARY_PATH. This matches what omnibus builds do.
        lib_path = os.path.join(prefix, 'lib')
        cmake_args += f' -DCMAKE_INSTALL_RPATH="{lib_path}"'
        cmake_args += ' -DCMAKE_INSTALL_RPATH_USE_LINK_PATH=TRUE'

    # Perform "out of the source build" in `rtloader_build_path` folder.
    try:
        os.makedirs(rtloader_build_path)
    except OSError as e:
        if e.errno == errno.EEXIST:
            pass
        else:
            raise

    with gitlab_section("Build rtloader", collapsed=True):
        ctx.run(f"cd {rtloader_build_path} && cmake {cmake_args} {get_rtloader_path()}", err_stream=sys.stdout)
        run_make_command(ctx)


@task
def clean(_):
    """
    Clean up CMake's cache and Bazel install artifacts under dev/.
    Necessary when the paths to some libraries found by CMake (for example Python) have changed on the system.
    """
    dev_path = get_dev_path()
    embedded_path = os.path.join(dev_path, "embedded")
    include_path = os.path.join(dev_path, "include")
    lib_path = os.path.join(dev_path, "lib")
    rtloader_build_path = get_rtloader_build_path()

    for p in [embedded_path, include_path, lib_path, rtloader_build_path]:
        try:
            shutil.rmtree(p)
            print(f"Successfully cleaned '{p}'")
        except FileNotFoundError:
            print(f"Nothing to clean up '{p}'")


@task
def install(ctx):
    with gitlab_section("Install rtloader", collapsed=True):
        run_make_command(ctx, "install")


@task
def install_with_bazel(ctx):
    """
    Install rtloader, Python, and Python's shared-library dependencies using Bazel.

    This is intended for local development alongside `agent.build`, as a way to leverage Bazel
    while we're still in a transitional stage. It installs an "embedded" environment which includes
    Python, under `dev/embedded`.

    Returns the embedded path.
    """
    dev_path = get_dev_path()

    if sys.platform == "win32":
        bin_dir = os.path.join(os.path.dirname(os.path.abspath(dev_path)), "bin")

        # Put static rtloader lib where the go build expects to find it
        bazel(
            ctx,
            "run",
            "//rtloader:install_static",
            "--",
            f"--destdir={os.path.join(get_rtloader_build_path(), 'rtloader')}",
        )

        # Install rtloader and cpython where the Agent expects them at runtime
        bazel(ctx, "run", "//rtloader:install", "--", f"--destdir={os.path.dirname(bin_dir)}")
        bazel(ctx, "run", "@cpython//:install", "--", f"--destdir={bin_dir}")

        return os.path.join(bin_dir, "embedded3")

    embedded = os.path.join(dev_path, "embedded")

    # Install rtloader + CPython + all Python deps
    bazel(ctx, "run", "//rtloader:install_python_env", "--", f"--destdir={dev_path}")

    # Patch RPATH in all installed binaries/libraries so they find their
    # dependencies under <embedded>/lib at runtime.
    files_to_patch = [
        f
        for f in ctx.run(
            f"find {embedded} -type f \\( -name '*.so' -o -name '*.so.*' -o -name '*.dylib'"
            f" -o -name '*.pc' -o -path '*/bin/*' \\)",
            hide="out",
        )
        .stdout.strip()
        .split("\n")
        if f
    ]
    if files_to_patch:
        bazel(ctx, "run", "//bazel/rules:replace_prefix", "--", "--prefix", embedded, *files_to_patch)

    return embedded


@task
def test(ctx):
    with gitlab_section("Run rtloader tests", collapsed=True):
        ctx.run(f"make -C {get_rtloader_build_path()}/test run", err_stream=sys.stdout)


@task
def format(ctx, raise_if_changed=False):
    with gitlab_section("Run clang-format on rtloader", collapsed=True):
        run_make_command(ctx, "clang-format")

    if raise_if_changed:
        changed_files = [line for line in ctx.run("git ls-files -m rtloader").stdout.strip().split("\n") if line]
        if len(changed_files) != 0:
            print("Following files were not correctly formated:")
            for f in changed_files:
                print(f"  - {f}")
            raise Exit(code=1)


@task
def generate_doc(ctx):
    """
    Generates the doxygen documentation, puts it in rtloader/doc, and logs doc errors/warnings.
    (rtloader/doc is hardcoded right now in the Doxyfile, as doxygen cannot take the output directory as argument)
    Logs all errors and warnings to <rtloader_path>/doxygen/errors.log and to the standard output.
    Returns 1 if errors were found (by default, doxygen returns 0 even if errors are present).
    """
    rtloader_path = get_rtloader_path()

    # Clean up Doxyfile options that are not supported on the version of Doxygen used
    result = ctx.run(f"doxygen -u '{rtloader_path}/doxygen/Doxyfile'", warn=True)
    if result.exited != 0:
        print("Fatal error encountered while trying to clean up the Doxyfile.")
        raise Exit(code=result.exited)

    # doxygen puts both errors and warnings in stderr
    result = ctx.run(
        "doxygen '{0}/doxygen/Doxyfile' 2>'{0}/doxygen/errors.log'".format(rtloader_path),  # noqa: UP032
        warn=True,
    )

    if result.exited != 0:
        print("Fatal error encountered while trying to generate documentation.")
        print(f"See {rtloader_path}/doxygen/errors.log for details.")
        raise Exit(code=result.exited)

    errors, warnings = [], []

    def flushentry(entry):
        if 'error:' in entry:
            errors.append(entry)
        elif 'warning:' in entry:
            warnings.append(entry)

    # Separate warnings from errors
    with open(f"{rtloader_path}/doxygen/errors.log") as errfile:
        currententry = ""
        for line in errfile.readlines():
            if 'error:' in line or 'warning:' in line:  # We get to a new entry, flush current one
                flushentry(currententry)
                currententry = ""

            currententry += line

        flushentry(currententry)  # Flush last entry

        print("\033[93m{}\033[0m".format("\n".join(warnings)))  # noqa: FS002
        print("\033[91m{}\033[0m".format("\n".join(errors)))  # noqa: FS002
        print(f"Found {len(errors)} error(s) and {len(warnings)} warning(s) while generating documentation.")
        print(f"The full list is available in {rtloader_path}/doxygen/errors.log.")

    # Exit with non-zero code if an error has been found
    if len(errors) > 0:
        raise Exit(code=1)
