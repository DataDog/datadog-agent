"""
msi namespaced tasks
"""


import glob
import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from tasks.ssm import get_pfx_pass, get_signing_cert
from tasks.utils import get_version, load_release_versions

# constants
OUTPUT_PATH = os.path.join(os.getcwd(), "omnibus", "pkg")
AGENT_TAG = "datadog/agent:master"
SOURCE_ROOT_DIR = os.path.join(os.getcwd(), "tools", "windows", "DatadogAgentInstaller")
BUILD_ROOT_DIR = os.path.join('C:\\', "dev", "msi", "DatadogAgentInstaller")
BUILD_SOURCE_DIR = os.path.join(BUILD_ROOT_DIR, "src")
BUILD_OUTPUT_DIR = os.path.join(BUILD_ROOT_DIR, "output")

NUGET_PACKAGES_DIR = os.path.join(BUILD_ROOT_DIR, 'packages')
NUGET_CONFIG_FILE = os.path.join(BUILD_ROOT_DIR, 'NuGet.config')
NUGET_CONFIG_BASE = '''<?xml version="1.0" encoding="utf-8"?>
<configuration>
</configuration>
'''


def _get_vs_build_command(cmd, vstudio_root=None):
    if not os.getenv("VCINSTALLDIR"):
        print("VC Not installed in environment; checking other locations")
        vsroot = vstudio_root or os.getenv('VSTUDIO_ROOT')
        if not vsroot:
            print("Must have visual studio installed")
            raise Exit(code=2)
        batchfile = "vcvars64.bat"
        vs_env_bat = f'{vsroot}\\VC\\Auxiliary\\Build\\{batchfile}'
        cmd = f'call "{vs_env_bat}" && {cmd}'
    return cmd


def _get_env(ctx, major_version='7', python_runtimes='3', release_version='nightly'):
    env = load_release_versions(ctx, release_version)

    env['PACKAGE_VERSION'] = get_version(
        ctx, include_git=True, url_safe=True, major_version=major_version, include_pipeline_id=True
    )
    env['PY_RUNTIMES'] = python_runtimes
    env['AGENT_INSTALLER_OUTPUT_DIR'] = f'{BUILD_OUTPUT_DIR}'
    env['NUGET_PACKAGES_DIR'] = f'{NUGET_PACKAGES_DIR}'
    return env


def _build(
    ctx,
    project='',
    vstudio_root=None,
    arch="x64",
    major_version='7',
    python_runtimes='3',
    release_version='nightly',
    debug=False,
):
    """
    Build the MSI installer builder, i.e. the program that can build an MSI
    """
    if sys.platform != 'win32':
        print("Building the MSI installer is only for available on Windows")
        raise Exit(code=1)

    env = _get_env(ctx, major_version, python_runtimes, release_version)
    print(f"arch is {arch}")

    cmd = ""
    configuration = "Release"
    if debug:
        configuration = "Debug"

    # Copy source to build dir
    # Hyper-v has a bug that causes the host's vmwp.exe to hold file locks indefinitely,
    # preventing the build from overwriting output files. To work around this copy the
    # source into the container, build on the container FS, then copy the output
    # back to the mount.
    try:
        ctx.run(f'robocopy {SOURCE_ROOT_DIR} {BUILD_SOURCE_DIR} /MIR /XF packages embedded3.COMPRESSED', hide=True)
    except UnexpectedExit as e:
        # robocopy can return non-zero success codes
        # Per https://ss64.com/nt/robocopy-exit.html
        # An Exit Code of 0-7 is success and any value >= 8 indicates that there was at least one failure during the copy operation.
        if e.result.return_code >= 8:
            # returned an error code, reraise exception
            raise

    # Create NuGet.config to set packages dir
    with open(NUGET_CONFIG_FILE, 'w') as f:
        f.write(NUGET_CONFIG_BASE)
    ctx.run(f'nuget config -set repositoryPath={NUGET_PACKAGES_DIR} -configfile {NUGET_CONFIG_FILE}')

    # Construct build command line
    cmd = _get_vs_build_command(
        f'cd {BUILD_SOURCE_DIR} && nuget restore && msbuild {project} /p:Configuration={configuration} /p:Platform="x64"',
        vstudio_root,
    )
    print(f"Build Command: {cmd}")

    # Try to run the command 3 times to alleviate transient
    # network failures
    succeeded = ctx.run(cmd, warn=True, env=env)
    if not succeeded:
        raise Exit("Failed to build the installer builder.", code=1)


def build_out_dir(arch, configuration):
    """
    Return the build output directory specific to this @arch and @configuration
    """
    return os.path.join(BUILD_OUTPUT_DIR, 'bin', arch, configuration)


@task
def build(
    ctx, vstudio_root=None, arch="x64", major_version='7', python_runtimes='3', release_version='nightly', debug=False
):
    """
    Build the MSI installer for the agent
    """
    # Build the builder executable
    _build(
        ctx,
        vstudio_root=vstudio_root,
        arch=arch,
        major_version=major_version,
        python_runtimes=python_runtimes,
        release_version=release_version,
        debug=debug,
    )
    configuration = "Release"
    if debug:
        configuration = "Debug"
    build_outdir = build_out_dir(arch, configuration)

    # sign build output that will be included in the installer MSI
    dd_wcs_enabled = os.environ.get('SIGN_WINDOWS_DD_WCS')
    if dd_wcs_enabled:
        for f in [os.path.join(build_outdir, 'CustomActions.dll')]:
            ctx.run(f'dd-wcs sign {f}')

    # Run the builder to produce the MSI
    env = _get_env(ctx, major_version, python_runtimes, release_version)
    # Set an env var to tell WixSetup.exe where to put the output MSI
    env['AGENT_MSI_OUTDIR'] = build_outdir
    succeeded = ctx.run(
        f'cd {BUILD_SOURCE_DIR}\\WixSetup && {build_outdir}\\WixSetup.exe',
        warn=True,
        env=env,
    )
    if not succeeded:
        raise Exit("Failed to build the MSI installer.", code=1)

    out_file = os.path.join(build_outdir, f"datadog-agent-ng-{env['PACKAGE_VERSION']}-1-x86_64.msi")

    # sign the MSI
    dd_wcs_enabled = os.environ.get('SIGN_WINDOWS_DD_WCS')
    if dd_wcs_enabled:
        ctx.run(f'dd-wcs sign {out_file}')

    # And copy it to the output path as a build artifact
    shutil.copy2(out_file, OUTPUT_PATH)

    # if the optional upgrade test helper exists then copy that too
    optional_output = os.path.join(build_outdir, "datadog-agent-ng-7.43.0~rc.3+git.485.14b9337-1-x86_64.msi")
    if os.path.exists(optional_output):
        shutil.copy2(optional_output, OUTPUT_PATH)


@task
def test(
    ctx, vstudio_root=None, arch="x64", major_version='7', python_runtimes='3', release_version='nightly', debug=False
):
    """
    Run the unit test for the MSI installer for the agent
    """
    _build(
        ctx,
        vstudio_root=vstudio_root,
        arch=arch,
        major_version=major_version,
        python_runtimes=python_runtimes,
        release_version=release_version,
        debug=debug,
    )
    configuration = "Release"
    if debug:
        configuration = "Debug"
    env = _get_env(ctx, major_version, python_runtimes, release_version)

    # Generate the config file
    build_outdir = build_out_dir(arch, configuration)
    if not ctx.run(
        f'inv -e generate-config --build-type="agent-py2py3" --output-file="{build_outdir}\\datadog.yaml"',
        warn=True,
        env=env,
    ):
        raise Exit("Could not generate test datadog.yaml file")

    # Run the tests
    if not ctx.run(
        f'dotnet test {build_outdir}\\CustomActions.Tests.dll', warn=True, env=env
    ):
        raise Exit(code=1)

    if not ctx.run(
        f'dotnet test {build_outdir}\\WixSetup.Tests.dll', warn=True, env=env
    ):
        raise Exit(code=1)
