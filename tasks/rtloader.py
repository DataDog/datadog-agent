"""
RtLoader namespaced tasks
"""
import os

from invoke import task
from invoke.exceptions import Exit

def get_rtloader_path():
    here = os.path.abspath(os.path.dirname(__file__))
    return os.path.abspath(os.path.join(here, '..', 'rtloader'))

def clear_cmake_cache(rtloader_path, settings):
    """
    CMake is not regenerated when we change an option. This function detect the
    current cmake settings and remove the cache if they have change to retrigger
    a cmake build.
    """
    cmake_cache = os.path.join(rtloader_path, "CMakeCache.txt")
    if not os.path.exists(cmake_cache):
        return

    settings = settings.copy()
    with open(cmake_cache) as cache:
        for line in cache.readlines():
            for key, value in settings.items():
                if line.strip() == key + "=" + value:
                    settings.pop(key)

    if settings:
        os.remove(cmake_cache)

@task
def build(ctx, install_prefix=None, python_runtimes=None, cmake_options=''):
    rtloader_path = get_rtloader_path()

    here = os.path.abspath(os.path.dirname(__file__))
    dev_path = os.path.join(here, '..', 'dev')

    cmake_args = cmake_options + " -DBUILD_DEMO:BOOL=OFF -DCMAKE_INSTALL_PREFIX:PATH={}".format(install_prefix or dev_path)

    python_runtimes = python_runtimes or os.environ.get("PYTHON_RUNTIMES") or "2"
    python_runtimes = python_runtimes.split(',')

    settings = {
            "DISABLE_PYTHON2:BOOL": "OFF",
            "DISABLE_PYTHON3:BOOL": "OFF"
            }
    if '2' not in python_runtimes:
        settings["DISABLE_PYTHON2:BOOL"] = "ON"
    if '3' not in python_runtimes:
        settings["DISABLE_PYTHON3:BOOL"] = "ON"

    # clear cmake cache if settings have changed since the last build
    clear_cmake_cache(rtloader_path, settings)

    for option, value in settings.items():
        cmake_args += " -D{}={} ".format(option, value)

    ctx.run("cd {} && cmake {} .".format(rtloader_path, cmake_args))
    ctx.run("make -C {}".format(rtloader_path))

@task
def install(ctx):
    ctx.run("make -C {} install".format(get_rtloader_path()))

@task
def test(ctx):
    ctx.run("make -C {}/test run".format(get_rtloader_path()))

@task
def format(ctx, raise_if_changed=False):
    ctx.run("make -C {} clang-format".format(get_rtloader_path()))

    if raise_if_changed:
        changed_files = [line for line in ctx.run("git ls-files -m rtloader").stdout.strip().split("\n") if line]
        if len(changed_files) != 0:
            print("Following files were not correctly formated:")
            for f in changed_files:
                print("  - {}".format(f))
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

    # doxygen puts both errors and warnings in stderr
    result = ctx.run("doxygen '{0}/doxygen/Doxyfile' 2>'{0}/doxygen/errors.log'".format(rtloader_path), warn=True)

    if result.exited != 0:
        print("Fatal error encountered while trying to generate documentation.")
        print("See {0}/doxygen/errors.log for details.".format(rtloader_path))
        raise Exit(code=result.exited)

    errors, warnings = [], []

    def flushentry(entry):
        if 'error:' in currententry:
            errors.append(currententry)
        elif 'warning:' in currententry:
            warnings.append(currententry)

    # Separate warnings from errors
    with open("{}/doxygen/errors.log".format(rtloader_path)) as errfile:
        currententry = ""
        for line in errfile.readlines():
            if 'error:' in line or 'warning:' in line: # We get to a new entry, flush current one
                flushentry(currententry)
                currententry = ""

            currententry += line

        flushentry(currententry) # Flush last entry

        print("\033[93m{}\033[0m".format("\n".join(warnings)))
        print("\033[91m{}\033[0m".format("\n".join(errors)))
        print("Found {} error(s) and {} warning(s) while generating documentation.".format(len(errors), len(warnings)))
        print("The full list is available in {}/doxygen/errors.log.".format(rtloader_path))

    # Exit with non-zero code if an error has been found
    if len(errors) > 0:
        raise Exit(code=1)
