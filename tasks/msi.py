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
from tasks.utils import get_version

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


@task
def build(ctx, vstudio_root=None, arch="x64", major_version='7', python_runtimes='3', debug=False):
    """
    Build the MSI installer for the agent
    """

    if sys.platform != 'win32':
        print("Building the MSI installer is only for available on Windows")
        raise Exit(code=1)

    env = {}

    env['PACKAGE_VERSION'] = get_version(
        ctx, include_git=True, url_safe=True, major_version=major_version, include_pipeline_id=True
    )
    env['PY_RUNTIMES'] = python_runtimes
    if os.environ.get('SIGN_WINDOWS'):
        # get certificate and password from ssm
        pfxfile = get_signing_cert(ctx)
        pfxpass = get_pfx_pass(ctx)
        env['SIGN_PFX'] = str(pfxfile)
        env['SIGN_PFX_PW'] = str(pfxpass)

    env['AGENT_INSTALLER_OUTPUT_DIR'] = f'{BUILD_OUTPUT_DIR}'
    env['NUGET_PACKAGES_DIR'] = f'{NUGET_PACKAGES_DIR}'
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
    if not os.getenv("VCINSTALLDIR"):
        print("VC Not installed in environment; checking other locations")

        vsroot = vstudio_root or os.getenv('VSTUDIO_ROOT')
        if not vsroot:
            print("Must have visual studio installed")
            raise Exit(code=2)
        batchfile = "vcvars64.bat"
        if arch == "x86":
            print("Only 64 bit Windows is supported.")
            raise Exit(code=3)
        vs_env_bat = f'{vsroot}\\VC\\Auxiliary\\Build\\{batchfile}'
        cmd = f'call "{vs_env_bat}" && cd {BUILD_SOURCE_DIR} && nuget restore && msbuild /p:Configuration={configuration} /p:Platform="x64"'
    else:
        cmd = f'cd {BUILD_SOURCE_DIR} && nuget restore && msbuild {BUILD_SOURCE_DIR} /p:Configuration={configuration} /p:Platform="x64"'

    print(f"Build Command: {cmd}")

    # Try to run the command 3 times to alleviate transient
    # network failures
    succeeded = ctx.run(cmd, warn=True, env=env)
    if not succeeded:
        raise Exit("Failed to build the MSI installer.", code=1)

    for artefact in glob.glob(f'{BUILD_SOURCE_DIR}\\WixSetup\\*.msi'):
        shutil.copy2(artefact, OUTPUT_PATH)
