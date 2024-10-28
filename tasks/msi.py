"""
msi namespaced tasks
"""

import hashlib
import mmap
import os
import shutil
import sys
from contextlib import contextmanager

from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from tasks.libs.common.utils import download_to_tempfile, timed
from tasks.libs.releasing.version import get_version, load_release_versions

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


def _get_env(ctx, major_version='7', release_version='nightly'):
    env = load_release_versions(ctx, release_version)

    env['PACKAGE_VERSION'] = get_version(
        ctx, include_git=True, url_safe=True, major_version=major_version, include_pipeline_id=True
    )
    env['AGENT_INSTALLER_OUTPUT_DIR'] = f'{BUILD_OUTPUT_DIR}'
    env['NUGET_PACKAGES_DIR'] = f'{NUGET_PACKAGES_DIR}'
    return env


def _msbuild_configuration(debug=False):
    return "Debug" if debug else "Release"


def _fix_makesfxca_dll(path):
    """
    Zero out the certificate data directory table entry on the PE file at @path

    MakeSfxCA.exe packages managed custom actions by bundling them into the sfxca.dll
    that ships with the WiX toolset. In WiX 11.2 sfxca.dll was shipping with a digital
    signature. When MakeSfxCA.exe copies the VERSION_INFO resource from the embedded
    CA DLL into sfxca.dll it does not properly update the certificate data directory
    offset, resulting in it pointing to garbage data. Since the certificate table looks
    corrupted tools like signtool/jsign will throw an error when trying to sign the output file.
    https://github.com/wixtoolset/issues/issues/6089

    Zero-ing out the certificate data directory table entry allows signtool/jsign to create a new
    certificate table in the PE file.

    This may be able to be removed if we upgrade to a later version of the WiX toolset.
    """

    def intval(data, offset, size):
        return int.from_bytes(data[offset : offset + size], 'little')

    def word(data, offset):
        return intval(data, offset, 2)

    def dword(data, offset):
        return intval(data, offset, 4)

    # offsets and magic numbers from
    # https://learn.microsoft.com/en-us/windows/win32/debug/pe-format
    with open(path, 'r+b') as fd, mmap.mmap(fd.fileno(), 0) as pe_data:
        # verify DOS magic
        if pe_data[0:3] != b'MZ\x90':
            raise Exit("Invalid DOS magic")
        # get offset to PE/NT header
        e_lfanew = dword(pe_data, 0x3C)
        # verify PE magic
        if dword(pe_data, e_lfanew) != dword(b'PE\x00\x00', 0):
            raise Exit("Invalid PE magic")
        # Check OptionalHeader magic (it affects the data directory base offset)
        OptionalHeader = e_lfanew + 0x18
        magic = word(pe_data, OptionalHeader)
        if magic == 0x010B:
            # PE32
            DataDirectory = OptionalHeader + 96
        elif magic == 0x20B:
            # PE32+
            DataDirectory = OptionalHeader + 112
        else:
            raise Exit(f"Invalid magic: {hex(magic)}")
        # calculate offset to the certificate table data directory entry
        ddentry_size = 8
        certificatetable_index = 4
        certificatetable = DataDirectory + certificatetable_index * ddentry_size
        ct_offset = dword(pe_data, certificatetable)
        ct_size = dword(pe_data, certificatetable + 4)
        if ct_offset == 0 and ct_size == 0:
            # no change necessary
            return
        print(
            f"{path}: zeroing out certificate table directory entry {ct_offset:x},{ct_size:x} at offset {certificatetable:x}"
        )
        # zero out the certificate table data directory entry
        pe_data[certificatetable : certificatetable + ddentry_size] = b'\x00' * ddentry_size


def sign_file(ctx, path, force=False):
    dd_wcs_enabled = os.environ.get('SIGN_WINDOWS_DD_WCS')
    if dd_wcs_enabled or force:
        return ctx.run(f'dd-wcs sign "{path}"')


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
    _fix_makesfxca_dll(os.path.join(outdir, ca_dll))
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


@task
def build(ctx, vstudio_root=None, arch="x64", major_version='7', release_version='nightly', debug=False):
    """
    Build the MSI installer for the agent
    """
    env = _get_env(ctx, major_version, release_version)
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
        msi_name = f"datadog-agent-{env['PACKAGE_VERSION']}-1-x86_64"
        _build_msi(ctx, env, build_outdir, msi_name, DATADOG_AGENT_MSI_ALLOW_LIST)

        # And copy it to the final output path as a build artifact
        shutil.copy2(os.path.join(build_outdir, msi_name + '.msi'), OUTPUT_PATH)

    # if the optional upgrade test helper exists then build that too
    optional_name = "datadog-agent-7.43.0~rc.3+git.485.14b9337-1-x86_64"
    if os.path.exists(os.path.join(build_outdir, optional_name + ".wxs")):
        with timed("Building optional MSI"):
            _build_msi(ctx, env, build_outdir, optional_name, DATADOG_AGENT_MSI_ALLOW_LIST)
            shutil.copy2(os.path.join(build_outdir, optional_name + '.msi'), OUTPUT_PATH)


@task
def build_installer(ctx, vstudio_root=None, arch="x64", debug=False):
    """
    Build the MSI installer for the agent
    """
    env = {}
    env['OMNIBUS_TARGET'] = 'installer'
    env['PACKAGE_VERSION'] = get_version(
        ctx, include_git=True, url_safe=True, major_version="7", include_pipeline_id=True
    )
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
def test(ctx, vstudio_root=None, arch="x64", major_version='7', release_version='nightly', debug=False):
    """
    Run the unit test for the MSI installer for the agent
    """
    env = _get_env(ctx, major_version, release_version)
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
        f'inv -e agent.generate-config --build-type="agent-py2py3" --output-file="{build_outdir}\\datadog.yaml"',
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
def validate_msi(_, allowlist, msi=None):
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


def get_msm_info(ctx, release_version):
    """
    Get the merge module info from the release.json for the given release_version
    """
    env = load_release_versions(ctx, release_version)
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
    if 'WINDOWS_APMINJECT_VERSION' in env:
        info = {
            'filename': 'ddapminstall.msm',
            'build': env['WINDOWS_APMINJECT_MODULE'],
            'version': env['WINDOWS_APMINJECT_VERSION'],
            'shasum': env['WINDOWS_APMINJECT_SHASUM'],
        }
        info['url'] = f"{base_url}/{info['build']}/ddapminstall-{info['version']}.msm"
        msm_info['APMINJECT'] = info
    return msm_info


@task(
    iterable=['drivers'],
    help={
        'drivers': 'List of drivers to fetch (default: DDNPM, DDPROCMON, APMINJECT)',
        'release_version': 'Release version to fetch drivers from (default: nightly-a7)',
    },
)
def fetch_driver_msm(ctx, drivers=None, release_version=None):
    """
    Fetch the driver merge modules (.msm) that are consumed by the Agent MSI.

    Defaults to the versions provided in the @release_version section of release.json
    """
    ALLOWED_DRIVERS = ['DDNPM', 'DDPROCMON', 'APMINJECT']
    if not release_version:
        release_version = 'nightly-a7'

    msm_info = get_msm_info(ctx, release_version)
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
