"""
msi namespaced tasks
"""


import glob
import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.ssm import get_pfx_pass, get_signing_cert
from tasks.utils import get_version

# constants
OUTPUT_PATH = os.path.join(os.getcwd(), "omnibus", "pkg")
AGENT_TAG = "datadog/agent:master"
ROOT_DIR = os.path.join(os.getcwd(), "tools", "windows", "DatadogAgentInstaller")


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

    print(f"arch is {arch}")

    cmd = ""
    configuration = "Release"
    if debug:
        configuration = "Debug"

    if not os.getenv("VCINSTALLDIR"):
        print("VC Not installed in environment; checking other locations")

        vsroot = vstudio_root or os.getenv('VSTUDIO_ROOT')
        if not vsroot:
            print("Must have visual studio installed")
            raise Exit(code=2)
        batchfile = "vcvars64.bat"
        if arch == "x86":
            batchfile = "vcvars32.bat"
        vs_env_bat = f'{vsroot}\\VC\\Auxiliary\\Build\\{batchfile}'
        cmd = f'call "{vs_env_bat}" && cd {ROOT_DIR} && nuget restore && msbuild /p:Configuration={configuration} /p:Platform="Any CPU"'
    else:
        cmd = f'cd {ROOT_DIR} && nuget restore && msbuild {ROOT_DIR} /p:Configuration={configuration} /p:Platform="Any CPU"'

    print(f"Build Command: {cmd}")

    # Try to run the command 3 times to alleviate transient
    # network failures
    succeeded = ctx.run(cmd, warn=True, env=env)
    if not succeeded:
        raise Exit("Failed to build the MSI installer.", code=1)

    for artefact in glob.glob(f'{ROOT_DIR}\\WixSetup\\*.msi'):
        shutil.copy2(artefact, OUTPUT_PATH)
