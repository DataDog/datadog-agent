"""
msi namespaced tasks
"""

import hashlib
import os
import re
import shutil
import sys
import tempfile
import zipfile
from contextlib import contextmanager
from pathlib import Path

from gitlab.v4.objects import Project
from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.utils import download_to_tempfile, running_in_ci, timed
from tasks.libs.dependencies import get_effective_dependencies_env
from tasks.libs.releasing.version import VERSION_RE, _create_version_from_match, get_version

# Windows only import
try:
    import msilib
except ImportError:
    if sys.platform == "win32":
        raise
    msilib = None

# constants
OUTPUT_PATH = os.path.join(os.getcwd(), "omnibus", "pkg")
AGENT_TAG = "datadog/agent:master"
SOURCE_ROOT_DIR = os.path.join(os.getcwd(), "tools", "windows", "DatadogAgentInstaller")
BUILD_ROOT_DIR = os.path.join('C:\\', "dev", "msi", "DatadogAgentInstaller")
BUILD_SOURCE_DIR = os.path.join(BUILD_ROOT_DIR, "src")
BUILD_OUTPUT_DIR = os.path.join(BUILD_ROOT_DIR, "output")
DDOT_ARTIFACT_DIR = os.path.join('C:\\', 'opt', 'datadog-agent-ddot')
# Match to AgentInstaller.cs BinSource
AGENT_BIN_SOURCE_DIR = os.path.join('C:\\', 'opt', 'datadog-agent', 'bin', 'agent')

NUGET_PACKAGES_DIR = os.path.join(BUILD_ROOT_DIR, 'packages')
NUGET_CONFIG_FILE = os.path.join(BUILD_ROOT_DIR, 'NuGet.config')
NUGET_CONFIG_BASE = '''<?xml version="1.0" encoding="utf-8"?>
<configuration>
</configuration>
'''

DATADOG_AGENT_MSI_ALLOW_LIST = [
    "APPLICATIONDATADIRECTORY",
    "EXAMPLECONFSLOCATION",
    "checks.d",
    "protected",
    "run",
    "logs",
    "ProgramMenuDatadog",
]

DATADOG_INSTALLER_MSI_ALLOW_LIST = [
    "APPLICATIONDATADIRECTORY",
    "DatadogInstallerData",
    "locks",
    "packages",
    "temp",
]


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


def _get_env(ctx, flavor=None):
    env = get_effective_dependencies_env()

    if flavor is None:
        flavor = os.getenv("AGENT_FLAVOR", "")

    env['PACKAGE_VERSION'] = get_version(ctx, include_git=True, url_safe=True, include_pipeline_id=True)
    env['AGENT_FLAVOR'] = flavor
    env['AGENT_INSTALLER_OUTPUT_DIR'] = BUILD_OUTPUT_DIR
    env['NUGET_PACKAGES_DIR'] = NUGET_PACKAGES_DIR
    env['AGENT_PRODUCT_NAME_SUFFIX'] = ""
    # Used for installation directories registry keys
    # https://github.com/openssl/openssl/blob/master/NOTES-WINDOWS.md#installation-directories
    # TODO: How best to configure the OpenSSL version?
    env['AGENT_OPENSSL_VERSION'] = "3.5"

    return env


def _is_fips_mode(env):
    return env['AGENT_FLAVOR'] == "fips"


def _msbuild_configuration(debug=False):
    return "Debug" if debug else "Release"


def sign_file(ctx, path, force=False):
    dd_wcs_enabled = os.environ.get('SIGN_WINDOWS_DD_WCS')
    if dd_wcs_enabled or force:
        return ctx.run(f'dd-wcs sign "{path}"')


def _ensure_wix_tools(ctx):
    """
    Ensure WiX 5.x dotnet tools and required extensions are installed globally.
    This is required for WixSharp_wix4 which relies on the wix dotnet tool.
    WixSharp_wix4 supports WiX 4.x and 5.x.
    """
    if sys.platform != 'win32':
        return

    WIX_VERSION = "5.0.2"
    # These extensions are required for the MSI build:
    # - Netfx: .NET Framework detection
    # - Util: Utility elements (RemoveFolderEx, EventSource, ServiceConfig, FailWhenDeferred)
    # - UI: Standard UI dialogs
    REQUIRED_EXTENSIONS = [
        "WixToolset.Netfx.wixext",
        "WixToolset.Util.wixext",
        "WixToolset.UI.wixext",
    ]

    # Check if wix is installed globally
    # Note: .NET global tools are invoked directly by name, not with 'dotnet' prefix
    result = ctx.run('wix --version', warn=True, hide=True)
    if not result or result.return_code != 0:
        if running_in_ci():
            raise Exit(
                "WiX tools not found in CI.",
                code=1,
            )
        # Install WiX 5.x globally
        print(f"WiX tools not found. Installing WiX {WIX_VERSION} globally...")
        result = ctx.run(f'dotnet tool install --global wix --version {WIX_VERSION}', warn=True)
        if not result or result.return_code != 0:
            raise Exit(
                f"Failed to install WiX tools. Please install manually with: dotnet tool install --global wix --version {WIX_VERSION}",
                code=1,
            )
        print("WiX tools installed successfully")
    else:
        print("WiX tools found")

    # Check and install required WiX extensions
    _ensure_wix_extensions(ctx, REQUIRED_EXTENSIONS, WIX_VERSION)


def _ensure_wix_extensions(ctx, extensions, version):
    """
    Ensure required WiX extensions are installed with the correct version.

    WiX extensions must match the WiX toolset version. For example, WiX 5.0.2
    requires extensions version 5.0.2. Using mismatched versions (e.g., 6.0.2
    extensions with WiX 5.0.2) will cause build errors.
    """
    # Get list of installed extensions
    result = ctx.run('wix extension list -g', warn=True, hide=True)
    installed_extensions = {}
    if result and result.return_code == 0 and result.stdout:
        # Parse output like "WixToolset.Netfx.wixext 5.0.2" (space-separated)
        for line in result.stdout.strip().split('\n'):
            parts = line.strip().split()
            if len(parts) >= 2:
                ext_name, ext_version = parts[0], parts[1]
                installed_extensions[ext_name] = ext_version

    for ext in extensions:
        expected = f"{ext}/{version}"
        if ext in installed_extensions:
            if installed_extensions[ext] == version:
                print(f"WiX extension {expected} already installed")
                continue
            else:
                if running_in_ci():
                    raise Exit(
                        f"WiX extension {ext} has wrong version {installed_extensions[ext]} in CI, need {version}.",
                        code=1,
                    )
                # Wrong version installed - remove it first
                print(f"Removing incompatible WiX extension {ext}/{installed_extensions[ext]}...")
                ctx.run(f'wix extension remove -g {ext}', warn=True, hide=True)
        print(f"Installing WiX extension {expected}...")
        result = ctx.run(f'wix extension add -g {expected}', warn=True)
        if not result or result.return_code != 0:
            raise Exit(
                f"Failed to install WiX extension {expected}. Please install manually with: wix extension add -g {expected}",
                code=1,
            )


def _build(
    ctx,
    env,
    configuration='Release',
    project='',
    vstudio_root=None,
):
    """
    Build the MSI installer builder, i.e. the program that can build an MSI
    """
    if sys.platform != 'win32':
        print("Building the MSI installer is only for available on Windows")
        raise Exit(code=1)

    # Ensure WiX 4+ tools are available
    _ensure_wix_tools(ctx)

    cmd = ""

    # Copy source to build dir
    # Hyper-v has a bug that causes the host's vmwp.exe to hold file locks indefinitely,
    # preventing the build from overwriting output files. To work around this copy the
    # source into the container, build on the container FS, then copy the output
    # back to the mount.
    try:
        ctx.run(
            f'robocopy {SOURCE_ROOT_DIR} {BUILD_SOURCE_DIR} /MIR /XF *.COMPRESSED *.g.wxs *.msi *.exe /XD bin obj .vs cab cabcache packages',
            hide=True,
        )
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
        f'cd {BUILD_SOURCE_DIR} && msbuild {project} /restore /p:Configuration={configuration} /p:Platform="x64" /verbosity:minimal',
        vstudio_root,
    )
    print(f"Build Command: {cmd}")

    # Try to run the command 3 times to alleviate transient
    # network failures
    succeeded = ctx.run(cmd, warn=True, env=env, err_stream=sys.stdout)
    if not succeeded:
        raise Exit("Failed to build the installer builder.", code=1)


def build_out_dir(arch, configuration):
    """
    Return the build output directory specific to this @arch and @configuration
    """
    return os.path.join(BUILD_OUTPUT_DIR, 'bin', arch, configuration)


def _build_wxs(ctx, env, outdir, ca_dll):
    """
    Runs WixSetup.exe to generate the WXS and a batch file to build the MSI

    at this time wixsharp also runs makesfxca to package our custom action DLLS into CA.dll files.
    """
    wixsetup = f'{outdir}\\WixSetup.exe'
    if not os.path.exists(wixsetup):
        raise Exit(f"WXS builder not found: {wixsetup}")

    # Run the builder to produce the WXS
    # Set an env var to tell WixSetup.exe where to put the output
    env['AGENT_MSI_OUTDIR'] = outdir
    # Create a MSI build cmd, not the full MSI
    env["BUILD_MSI_CMD"] = "true"
    succeeded = ctx.run(
        f'cd {BUILD_SOURCE_DIR}\\WixSetup && {wixsetup}',
        warn=True,
        env=env,
    )
    if not succeeded:
        raise Exit("Failed to build the MSI WXS.", code=1)

    # sign the MakeSfxCA output files
    # If signing fails due to corrupted PE file / signature, it may be a regression caused by the makesfxca template DLL being previously signed before the embedded files were added to it. For more information refer to `_fix_makesfxca_dll` from `msi.py` in the git history.
    sign_file(ctx, os.path.join(outdir, ca_dll))


def _build_msi(ctx, env, outdir, name, allowlist):
    # Run the generated build command to build the MSI
    build_cmd = os.path.join(outdir, f"Build_{name}.cmd")
    if not os.path.exists(build_cmd):
        raise Exit(f"MSI build script not found: {build_cmd}")

    succeeded = ctx.run(
        f'cd {BUILD_SOURCE_DIR}\\WixSetup && {build_cmd}',
        warn=True,
        env=env,
    )
    if not succeeded:
        raise Exit("Failed to build the MSI installer.", code=1)

    out_file = os.path.join(outdir, f"{name}.msi")
    validate_msi(ctx, allowlist, out_file)
    sign_file(ctx, out_file)


def _build_datadog_interop(ctx, env, configuration, arch, vstudio_root):
    """Build DatadogInterop DLL using the standard build command."""
    datadog_interop_sln = os.path.join(os.getcwd(), "tools", "windows", "DatadogInterop", "DatadogInterop.sln")
    cmd = _get_vs_build_command(
        f'msbuild "{datadog_interop_sln}" /p:Configuration={configuration} /p:Platform="{arch}" /verbosity:minimal',
        vstudio_root,
    )
    print(f"Building DatadogInterop: {cmd}")
    succeeded = ctx.run(cmd, warn=True, env=env, err_stream=sys.stdout)
    if not succeeded:
        raise Exit("Failed to build DatadogInterop.", code=1)


@task
def build_datadog_interop(ctx, configuration="Release", arch="x64", vstudio_root=None, copy_to_root=True):
    """
    Build the libdatadog-interop.dll required for software inventory.

    This DLL provides interop functionality for MS Store apps collection on Windows.

    Args:
        configuration: Build configuration (Release or Debug)
        arch: Target architecture (x64)
        vstudio_root: Path to Visual Studio installation root
        copy_to_root: Whether to copy the DLL to the repository root for test access
    """
    if sys.platform != 'win32':
        print("Skipping DatadogInterop build on non-Windows platform")
        return

    env = get_effective_dependencies_env()
    _build_datadog_interop(ctx, env, configuration, arch, vstudio_root)

    if copy_to_root:
        # Copy the DLL to the repository root so it can be found during test execution
        dll_source = os.path.join(
            os.getcwd(), "tools", "windows", "DatadogInterop", arch, configuration, "libdatadog-interop.dll"
        )
        dll_dest = os.path.join(os.getcwd(), "libdatadog-interop.dll")

        if os.path.exists(dll_source):
            print(f"Copying DLL from {dll_source} to {dll_dest}")
            shutil.copy2(dll_source, dll_dest)
            print("Successfully built and copied libdatadog-interop.dll")
        else:
            print(f"Warning: Could not find built DLL at {dll_source}")


def _msi_output_name(env):
    if _is_fips_mode(env):
        return f"datadog-fips-agent-{env['AGENT_PRODUCT_NAME_SUFFIX']}{env['PACKAGE_VERSION']}-1-x86_64"
    else:
        return f"datadog-agent-{env['AGENT_PRODUCT_NAME_SUFFIX']}{env['PACKAGE_VERSION']}-1-x86_64"


@task
def build(
    ctx,
    vstudio_root=None,
    arch="x64",
    flavor=None,
    debug=False,
    build_upgrade=False,
):
    """
    Build the MSI installer for the agent
    """
    env = _get_env(ctx, flavor=flavor)
    env['OMNIBUS_TARGET'] = 'main'
    configuration = _msbuild_configuration(debug=debug)
    build_outdir = build_out_dir(arch, configuration)

    # Build the builder executable (WixSetup.exe)
    _build(
        ctx,
        env,
        configuration=configuration,
        vstudio_root=vstudio_root,
    )

    # Build libdatadog-interop.dll
    _build_datadog_interop(ctx, env, configuration, arch, vstudio_root)
    datadog_interop_output = os.path.join(
        os.getcwd(), "tools", "windows", "DatadogInterop", arch, configuration, "libdatadog-interop.dll"
    )
    shutil.copy2(datadog_interop_output, AGENT_BIN_SOURCE_DIR)
    sign_file(ctx, os.path.join(AGENT_BIN_SOURCE_DIR, 'libdatadog-interop.dll'))

    # sign build output that will be included in the installer MSI
    sign_file(ctx, os.path.join(build_outdir, 'CustomActions.dll'))
    sign_file(ctx, os.path.join(build_outdir, 'AgentCustomActions.dll'))

    # We embed this 7zip standalone binary in the installer, sign it too
    shutil.copy2('C:\\Program Files\\7-zip\\7zr.exe', AGENT_BIN_SOURCE_DIR)
    sign_file(ctx, os.path.join(AGENT_BIN_SOURCE_DIR, '7zr.exe'))

    # Run WixSetup.exe to generate the WXS and other input files
    with timed("Building WXS"):
        _build_wxs(
            ctx,
            env,
            build_outdir,
            'AgentCustomActions.CA.dll',
        )

    # Run WiX to turn the WXS into an MSI
    with timed("Building MSI"):
        msi_name = _msi_output_name(env)
        _build_msi(ctx, env, build_outdir, msi_name, DATADOG_AGENT_MSI_ALLOW_LIST)

        # And copy it to the final output path as a build artifact
        shutil.copy2(os.path.join(build_outdir, msi_name + '.msi'), OUTPUT_PATH)

    # Build the optional upgrade test helper
    if build_upgrade:
        print("Building optional upgrade test helper")
        upgrade_env = env.copy()
        version = _create_version_from_match(VERSION_RE.search(env['PACKAGE_VERSION']))
        next_version = version.next_version(bump_patch=True)
        upgrade_env['PACKAGE_VERSION'] = upgrade_env['PACKAGE_VERSION'].replace(str(version), str(next_version))
        upgrade_env['AGENT_PRODUCT_NAME_SUFFIX'] = "upgrade-test-"
        _build_wxs(
            ctx,
            upgrade_env,
            build_outdir,
            'AgentCustomActions.CA.dll',
        )
        msi_name = _msi_output_name(upgrade_env)
        print(os.path.join(build_outdir, msi_name + ".wxs"))
        with timed("Building optional MSI"):
            _build_msi(ctx, env, build_outdir, msi_name, DATADOG_AGENT_MSI_ALLOW_LIST)
            shutil.copy2(os.path.join(build_outdir, msi_name + '.msi'), OUTPUT_PATH)


@task
def build_installer(ctx, vstudio_root=None, arch="x64", debug=False):
    """
    Build the MSI installer for the agent
    """
    env = {}
    env['OMNIBUS_TARGET'] = 'installer'
    env['PACKAGE_VERSION'] = get_version(ctx, include_git=True, url_safe=True, include_pipeline_id=True)
    env['NUGET_PACKAGES_DIR'] = f'{NUGET_PACKAGES_DIR}'
    env['AGENT_INSTALLER_OUTPUT_DIR'] = f'{BUILD_OUTPUT_DIR}'
    configuration = _msbuild_configuration(debug=debug)
    build_outdir = build_out_dir(arch, configuration)

    # Build the builder executable (WixSetup.exe)
    _build(
        ctx,
        env,
        configuration=configuration,
        vstudio_root=vstudio_root,
    )

    # sign build output that will be included in the installer MSI
    sign_file(ctx, os.path.join(build_outdir, 'CustomActions.dll'))
    sign_file(ctx, os.path.join(build_outdir, 'InstallerCustomActions.dll'))

    # Run WixSetup.exe to generate the WXS and other input files
    with timed("Building WXS"):
        _build_wxs(ctx, env, build_outdir, 'InstallerCustomActions.CA.dll')

    with timed("Building MSI"):
        msi_name = f"datadog-installer-{env['PACKAGE_VERSION']}-1-x86_64"
        _build_msi(ctx, env, build_outdir, msi_name, DATADOG_INSTALLER_MSI_ALLOW_LIST)

        # And copy it to the final output path as a build artifact
        shutil.copy2(os.path.join(build_outdir, msi_name + '.msi'), OUTPUT_PATH)


@task
def test(ctx, vstudio_root=None, arch="x64", debug=False):
    """
    Run the unit test for the MSI installer for the agent
    """
    env = _get_env(ctx)
    configuration = _msbuild_configuration(debug=debug)
    build_outdir = build_out_dir(arch, configuration)

    _build(
        ctx,
        env,
        configuration=configuration,
        vstudio_root=vstudio_root,
    )

    # Generate the config file
    if not ctx.run(
        f'dda inv -- -e agent.generate-config --build-type="agent-py3" --output-file="{build_outdir}\\datadog.yaml"',
        warn=True,
        env=env,
    ):
        raise Exit("Could not generate test datadog.yaml file")

    # Run the tests
    if not ctx.run(f'dotnet test {build_outdir}\\CustomActions.Tests.dll', warn=True, env=env):
        raise Exit(code=1)

    if not ctx.run(f'dotnet test {build_outdir}\\WixSetup.Tests.dll', warn=True, env=env):
        raise Exit(code=1)


def validate_msi_createfolder_table(db, allowlist):
    """
    Checks that the CreateFolder MSI table only contains certain directories.

    We found that WiX# was causing directories like TARGETDIR (C:\\) and ProgramFiles64Folder
    (C:\\Program Files\\) to end up in the CrateFolder MSI table. Then because MSI.dll CreateFolder rollback
    uses the obsolete SetFileSecurityW function the AI DACL flag is removed from those directories
    on rollback.
    https://github.com/oleg-shilo/wixsharp/issues/1336

    If you think you need to add a new directory to this list, perform the following checks:
    * Ensure the directory and its parents are deleted or persisted on uninstall as expected
    * If the directory may be persisted after rollback, check if AI flag is removed and consider if that's okay or not

    TODO: We don't want the AI flag to be removed from the directories in the allow list either, but
          this behavior was also present in the original installer so leave them for now.
    """

    # Skip if CreateFolder table does not exist
    with MsiClosing(db.OpenView("Select `Name` From `_Tables`")) as view:
        view.Execute(None)
        record = view.Fetch()
        tables = set()
        while record:
            tables.add(record.GetString(1))
            record = view.Fetch()
        if "CreateFolder" not in tables:
            print("skipping validation, CreateFolder table not found in MSI")
            return

    print("Validating MSI CreateFolder table")
    with MsiClosing(db.OpenView("Select Directory_ FROM CreateFolder")) as view:
        view.Execute(None)
        record = view.Fetch()
        unexpected = set()
        while record:
            directory = record.GetString(1)
            if directory not in allowlist:
                unexpected.add(directory)
            record = view.Fetch()

    if unexpected:
        for directory in unexpected:
            print(f"Unexpected directory '{directory}' in MSI CreateFolder table")
        raise Exit(f"{len(unexpected)} unexpected directories in MSI CreateFolder table")


@task
def validate_msi(ctx, allowlist, msi=None):
    print("Validating MSI")
    ctx.run(f'wix msi validate "{msi}"')
    with MsiClosing(msilib.OpenDatabase(msi, msilib.MSIDBOPEN_READONLY)) as db:
        validate_msi_createfolder_table(db, allowlist)


@contextmanager
def MsiClosing(obj):
    """
    The MSI objects use Close() instead of close() so we can't use the built-in closing()
    """
    try:
        yield obj
    finally:
        obj.Close()


def get_msm_info(ctx):
    """
    Get the merge module info from the release.json
    """
    env = get_effective_dependencies_env()
    base_url = "https://s3.amazonaws.com/dd-windowsfilter/builds"
    msm_info = {}
    if 'WINDOWS_DDNPM_VERSION' in env:
        info = {
            'filename': 'DDNPM.msm',
            'build': env['WINDOWS_DDNPM_DRIVER'],
            'version': env['WINDOWS_DDNPM_VERSION'],
            'shasum': env['WINDOWS_DDNPM_SHASUM'],
        }
        info['url'] = f"{base_url}/{info['build']}/ddnpminstall-{info['version']}.msm"
        msm_info['DDNPM'] = info
    if 'WINDOWS_DDPROCMON_VERSION' in env:
        info = {
            'filename': 'DDPROCMON.msm',
            'build': env['WINDOWS_DDPROCMON_DRIVER'],
            'version': env['WINDOWS_DDPROCMON_VERSION'],
            'shasum': env['WINDOWS_DDPROCMON_SHASUM'],
        }
        info['url'] = f"{base_url}/{info['build']}/ddprocmoninstall-{info['version']}.msm"
        msm_info['DDPROCMON'] = info
    return msm_info


@task(
    iterable=['drivers'],
    help={
        'drivers': 'List of drivers to fetch (default: DDNPM, DDPROCMON)',
    },
)
def fetch_driver_msm(ctx, drivers=None):
    """
    Fetch the driver merge modules (.msm) that are consumed by the Agent MSI.

    Defaults to the versions provided in the dependencies section of release.json
    """
    ALLOWED_DRIVERS = ['DDNPM', 'DDPROCMON']

    msm_info = get_msm_info(ctx)
    if not drivers:
        # if user did not specify drivers, use the ones in the release.json
        drivers = msm_info.keys()

    for driver in drivers:
        driver = driver.upper()
        if driver not in ALLOWED_DRIVERS:
            raise Exit(f"Invalid driver: {driver}, choose from {ALLOWED_DRIVERS}")

        info = msm_info[driver]
        url = info['url']
        shasum = info['shasum']
        path = os.path.join(AGENT_BIN_SOURCE_DIR, info['filename'])

        # download from url with requests package
        checksum = hashlib.sha256()
        with download_to_tempfile(url, checksum) as tmp_path:
            # check sha256
            if checksum.hexdigest().lower() != shasum.lower():
                raise Exit(f"Checksum mismatch for {url}")
            # move to final path
            shutil.move(tmp_path, path)

        print(f"Updated {driver}")
        print(f"\t-> Downloaded {url} to {path}")


@task(
    help={
        'ref': 'The name of the ref (branch, tag) to fetch the latest artifacts from',
        'ddot': 'Also download the DDOT zip artifact (default: False)',
    },
)
def fetch_artifacts(ctx, ref: str | None = None, ddot: bool = False) -> None:
    """
    Initialize the build environment with artifacts from a ref (default: main)

    Example:
    dda inv msi.fetch-artifacts --ref main
    dda inv msi.fetch-artifacts --ref 7.66.x
    dda inv msi.fetch-artifacts --ref main --ddot
    """
    if ref is None:
        ref = 'main'

    project = get_gitlab_repo()

    with tempfile.TemporaryDirectory() as tmp_dir:
        download_latest_artifacts_for_ref(project, ref, tmp_dir)

        if ddot:
            download_latest_artifacts_for_ref(project, ref, tmp_dir, job='windows_zip_ddot_x64')

        tmp_dir_path = Path(tmp_dir)

        print(f"Downloaded artifacts to {tmp_dir_path}")

        # Recursively search for the zip files
        ddot_zips = list(tmp_dir_path.glob("**/datadog-agent-ddot-*x86_64.zip"))
        ddot_set = set(ddot_zips)
        agent_zips = [z for z in tmp_dir_path.glob("**/datadog-agent-*-x86_64.zip") if z not in ddot_set]
        installer_zips = list(tmp_dir_path.glob("**/datadog-installer-*-x86_64.zip"))

        print(f"Found {len(agent_zips)} agent zip files")
        print(f"Found {len(installer_zips)} installer zip files")
        if ddot:
            print(f"Found {len(ddot_zips)} DDOT zip files")

        if not agent_zips and not installer_zips:
            print("No zip files found. Directory contents:")
            for path in tmp_dir_path.glob("**/*"):
                if path.is_file():
                    print(f"  {path}")
            raise Exception("No zip files found in the downloaded artifacts")

        # Extract agent zips
        dest = Path(r'C:\opt\datadog-agent')
        dest.mkdir(parents=True, exist_ok=True)
        for zip_file in agent_zips:
            print(f"Extracting {zip_file} to {dest}")
            with zipfile.ZipFile(zip_file, "r") as zip_ref:
                zip_ref.extractall(dest)

        # Extract installer zips
        dest = Path(r'C:\opt\datadog-installer')
        dest.mkdir(parents=True, exist_ok=True)
        for zip_file in installer_zips:
            print(f"Extracting {zip_file} to {dest}")
            with zipfile.ZipFile(zip_file, "r") as zip_ref:
                zip_ref.extractall(dest)

        # Extract DDOT zips
        if ddot_zips:
            dest = Path(DDOT_ARTIFACT_DIR)
            dest.mkdir(parents=True, exist_ok=True)
            for zip_file in ddot_zips:
                print(f"Extracting {zip_file} to {dest}")
                with zipfile.ZipFile(zip_file, "r") as zip_ref:
                    zip_ref.extractall(dest)

        print("Extraction complete")

    # Delete stale embedded3.COMPRESSED so the next debug build re-compresses
    # from the fresh artifacts. In debug builds CompressedDir.cs skips
    # re-compression when the file already exists.
    compressed_file = os.path.join(BUILD_SOURCE_DIR, 'WixSetup', 'embedded3.COMPRESSED')
    if os.path.exists(compressed_file):
        print(f"Deleting stale compressed file: {compressed_file}")
        os.remove(compressed_file)


def download_latest_artifacts_for_ref(
    project: Project,
    ref_name: str,
    output_dir: str,
    job: str = 'windows_msi_and_bosh_zip_x64-a7',
) -> None:
    """
    Fetch the latest artifacts for a ref from gitlab and store them in the output directory
    """
    print(f"Downloading artifacts for branch {ref_name} (job: {job})")
    fd, tmp_path = tempfile.mkstemp()
    try:
        with os.fdopen(fd, "wb") as f:
            # fd will be closed by context manager, so we no longer need it
            fd = None

            # wrap write to satisfy type for action
            def writewrapper(b: bytes) -> None:
                f.write(b)

            project.artifacts.download(
                ref_name=ref_name,
                job=job,
                streamed=True,
                action=writewrapper,
            )
        print(f"Extracting artifacts to {output_dir}")
        with zipfile.ZipFile(tmp_path, "r") as zip_ref:
            zip_ref.extractall(output_dir)
    finally:
        if fd is not None:
            os.close(fd)
        if os.path.exists(tmp_path):
            os.remove(tmp_path)


@task(
    help={
        'msi_path': 'Path to the MSI or ZIP file (default: auto-detect in omnibus/pkg)',
        'output_dir': 'Output directory for the OCI tar (default: omnibus/pkg)',
        'source_type': "Source type - 'msi' or 'zip' (default: msi)",
        'ddot': 'Include the DDOT extension layer (auto-detected from fetch-artifacts --ddot)',
        'ddot_path': 'Explicit path to the extracted DDOT artifact directory (overrides auto-detect)',
    },
)
def package_oci(
    ctx,
    msi_path=None,
    output_dir=None,
    source_type="msi",
    ddot=False,
    ddot_path=None,
):
    """
    Create an OCI package from an MSI installer.

    Use --ddot to include the DDOT extension layer. The DDOT artifacts are
    auto-detected from the directory populated by fetch-artifacts --ddot.
    Use --ddot-path to override with an explicit directory.

    Example:
        dda inv msi.package-oci
        dda inv msi.package-oci --ddot
        dda inv msi.package-oci --ddot-path C:\\path\\to\\extracted-ddot

    Requires:
        datadog-package: Install from https://github.com/DataDog/datadog-package
            go install github.com/DataDog/datadog-packages/cmd/datadog-package@latest
    """
    import tempfile
    from pathlib import Path

    # Set defaults
    if output_dir is None:
        output_dir = OUTPUT_PATH

    # Determine package version
    package_version = None

    # Verify datadog-package is in PATH
    datadog_package_path = shutil.which("datadog-package")
    if datadog_package_path is None:
        print("datadog-package not found in PATH")
        print("To install datadog-package, run:")
        print("  go install github.com/DataDog/datadog-packages/cmd/datadog-package@latest")
        print("Or see: https://github.com/DataDog/datadog-packages")
        raise Exit(code=1)

    if msi_path is None and source_type == "msi":
        # Auto-detect: Get version from git and find matching MSI
        package_version = get_version(ctx, include_git=True, url_safe=True, include_pipeline_id=True)
        msi_pattern = f"datadog-agent-{package_version}-1-x86_64.msi"
        msi_files = list(Path(OUTPUT_PATH).glob(msi_pattern))
        if not msi_files:
            print(f"No MSI found matching pattern: {msi_pattern}")
            raise Exit(code=1)
        msi_path = str(msi_files[0])
        print(f"Found MSI: {msi_path}")
    elif msi_path is None and source_type == "zip":
        print("When source_type='zip', msi_path must be explicitly provided.")
        raise Exit(code=1)
    elif msi_path is not None:
        # MSI path provided: Extract version from filename
        if not os.path.exists(msi_path):
            print(f"MSI file not found: {msi_path}")
            raise Exit(code=1)
        # Verify file extension matches source type
        msi_file = Path(msi_path)
        file_ext = msi_file.suffix.lower()
        if source_type == "zip":
            expected_ext = ".zip"
        else:
            expected_ext = ".msi"

        if file_ext != expected_ext:
            print(f"Error: source_type='{source_type}' but file has extension '{file_ext}'")
            print(f"Expected a '{expected_ext}' file.")
            raise Exit(code=1)

        # Expected format: datadog-agent-{VERSION}-1-x86_64.msi
        msi_filename = os.path.basename(msi_path)
        pattern = rf'datadog-agent-(.+)-1-x86_64\{file_ext}$'
        version_match = re.search(pattern, msi_filename)

        if not version_match:
            print(f"Could not extract version from filename: {msi_filename}")
            print(f"Expected format: datadog-agent-{{VERSION}}-1-x86_64{file_ext}")
            raise Exit(code=1)

        package_version = version_match.group(1)
        print(f"Extracted version from MSI filename: {package_version}")

    # Resolve DDOT extension directory
    ddot_ext_dir = None
    if ddot_path is not None:
        if not os.path.isdir(ddot_path):
            print(f"DDOT directory not found: {ddot_path}")
            raise Exit(code=1)
        ddot_ext_dir = ddot_path
        print(f"Using DDOT directory: {ddot_ext_dir}")
    elif ddot:
        ddot_dir = Path(DDOT_ARTIFACT_DIR)
        if not ddot_dir.exists() or not any(ddot_dir.iterdir()):
            print(f"No DDOT artifacts found in {ddot_dir}")
            print("Run 'dda inv msi.fetch-artifacts --ddot' first, or provide --ddot-path.")
            raise Exit(code=1)
        ddot_ext_dir = str(ddot_dir)
        print(f"Auto-detected DDOT directory: {ddot_ext_dir}")

    installer_bin_path = 'C:\\opt\\datadog-installer\\datadog-installer.exe'
    if os.path.exists(installer_bin_path):
        print(f"Using installer binary: {installer_bin_path}")
    else:
        print(f"Installer binary not found: {installer_bin_path}")
        raise Exit(code=1)

    # Create temporary directory for input
    with tempfile.TemporaryDirectory() as src_dir:
        print(f"Using temporary directory: {src_dir}")

        # Handle different source types
        extra_flags = ""
        if source_type == "msi":
            # Copy MSI to temp directory
            print(f"Copying MSI to {src_dir}")
            shutil.copy2(msi_path, src_dir)
        elif source_type == "zip":
            # Extract ZIP to temp directory
            print(f"Extracting ZIP to {src_dir}")
            with zipfile.ZipFile(msi_path, "r") as zip_ref:
                zip_ref.extractall(src_dir)

            # Check for config directory
            config_dir = os.path.join(src_dir, "etc", "datadog-agent")
            if os.path.exists(config_dir):
                extra_flags = f"--configs {config_dir}"
        else:
            print(f"Unknown source type: {source_type}")
            raise Exit(code=1)

        if ddot_ext_dir:
            extra_flags += f' --extension ddot={ddot_ext_dir}'
        if installer_bin_path:
            extra_flags += f' --installer {installer_bin_path}'

        # Construct output path
        oci_output_path = os.path.join(output_dir, f"datadog-agent-{package_version}-1-windows-amd64.oci.tar")

        # Build the command
        cmd = (
            f'"{datadog_package_path}" create '
            f'--version "{package_version}-1" '
            f'--package "datadog-agent" '
            f'--os windows '
            f'--arch amd64 '
            f'--archive '
            f'--archive-path "{oci_output_path}" '
        )

        if extra_flags:
            cmd += f'{extra_flags} '

        cmd += f'"{src_dir}"'
        result = ctx.run(cmd, warn=True)

        if not result:
            print("Failed to create OCI package")
            raise Exit(code=1)

        if os.path.exists(oci_output_path):
            print(f"Successfully created OCI package: {oci_output_path}")
        else:
            print(f"OCI package not found at expected path: {oci_output_path}")
            raise Exit(code=1)
