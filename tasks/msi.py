"""
msi namespaced tasks
"""


import mmap
import os
import shutil
import sys
from contextlib import contextmanager

from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from tasks.utils import get_version, load_release_versions, timed

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

NUGET_PACKAGES_DIR = os.path.join(BUILD_ROOT_DIR, 'packages')
NUGET_CONFIG_FILE = os.path.join(BUILD_ROOT_DIR, 'NuGet.config')
NUGET_CONFIG_BASE = '''<?xml version="1.0" encoding="utf-8"?>
<configuration>
</configuration>
'''

BinFiles = r"C:\omnibus-ruby\src\datadog-agent\src\github.com\DataDog\datadog-agent\bin"
InstallerSource = r"C:\opt\datadog-agent"


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
        return ctx.run(f'dd-wcs sign {path}')


def _build(
    ctx,
    env,
    arch='x64',
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
            f'robocopy {SOURCE_ROOT_DIR} {BUILD_SOURCE_DIR} /MIR /XF cabcache packages embedded2.COMPRESSED embedded3.COMPRESSED',
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
        f'cd {BUILD_SOURCE_DIR} && msbuild {project} /restore /p:Configuration={configuration} /p:Platform="{arch}"',
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


def _build_wxs(ctx, env, outdir):
    """
    Runs WixSetup.exe to generate the WXS and other files to be included in the MSI
    """
    wixsetup = f'{outdir}\\WixSetup.exe'
    if not os.path.exists(wixsetup):
        raise Exit(f"WXS builder not found: {wixsetup}")

    # Run the builder to produce the WXS
    # Set an env var to tell WixSetup.exe where to put the output
    env['AGENT_MSI_OUTDIR'] = outdir
    succeeded = ctx.run(
        f'cd {BUILD_SOURCE_DIR}\\WixSetup && {wixsetup}',
        warn=True,
        env=env,
    )
    if not succeeded:
        raise Exit("Failed to build the MSI WXS.", code=1)

    # sign the MakeSfxCA output files
    _fix_makesfxca_dll(os.path.join(outdir, 'CustomActions.CA.dll'))
    sign_file(ctx, os.path.join(outdir, 'CustomActions.CA.dll'))


def _build_msi(ctx, env, outdir, name):
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
    validate_msi(ctx, out_file)
    sign_file(ctx, out_file)


def _python_signed_files(python_runtimes='3'):
    runtimes = python_runtimes.split(',')
    files = []

    if '3' in runtimes:
        for f in ['python.exe', 'python3.dll', 'python39.dll', 'pythonw.exe']:
            files.append(os.path.join(InstallerSource, 'embedded3', f))
    if '2' in runtimes:
        for f in ['python.exe', 'python27.dll', 'pythonw.exe']:
            files.append(os.path.join(InstallerSource, 'embedded2', f))

    return files


@task
def build(
    ctx, vstudio_root=None, arch="x64", major_version='7', python_runtimes='3', release_version='nightly', debug=False
):
    """
    Build the MSI installer for the agent
    """
    env = _get_env(ctx, major_version, python_runtimes, release_version)
    configuration = _msbuild_configuration(debug=debug)
    build_outdir = build_out_dir(arch, configuration)

    # Build the builder executable (WixSetup.exe)
    _build(
        ctx,
        env,
        arch=arch,
        configuration=configuration,
        vstudio_root=vstudio_root,
    )

    # sign build output that will be included in the installer MSI
    # NOTE: Most of the files in BinFiles are signed by the agent MSI omnibus task
    with timed("Signing files"):
        for f in [
            os.path.join(build_outdir, 'CustomActions.dll'),
            os.path.join(BinFiles, 'agent', 'ddtray.exe'),
        ] + _python_signed_files(python_runtimes=python_runtimes):
            sign_file(ctx, f)

    # Run WixSetup.exe to generate the WXS and other input files
    with timed("Building WXS"):
        _build_wxs(
            ctx,
            env,
            build_outdir,
        )

    # Run WiX to turn the WXS into an MSI
    with timed("Building MSI"):
        msi_name = f"datadog-agent-ng-{env['PACKAGE_VERSION']}-1-x86_64"
        _build_msi(
            ctx,
            env,
            build_outdir,
            msi_name,
        )

        # And copy it to the final output path as a build artifact
        shutil.copy2(os.path.join(build_outdir, msi_name + '.msi'), OUTPUT_PATH)

    # if the optional upgrade test helper exists then build that too
    optional_name = "datadog-agent-ng-7.43.0~rc.3+git.485.14b9337-1-x86_64"
    if os.path.exists(os.path.join(build_outdir, optional_name + ".wxs")):
        with timed("Building optional MSI"):
            _build_msi(
                ctx,
                env,
                build_outdir,
                optional_name,
            )
            shutil.copy2(os.path.join(build_outdir, optional_name + '.msi'), OUTPUT_PATH)


@task
def test(
    ctx, vstudio_root=None, arch="x64", major_version='7', python_runtimes='3', release_version='nightly', debug=False
):
    """
    Run the unit test for the MSI installer for the agent
    """
    env = _get_env(ctx, major_version, python_runtimes, release_version)
    configuration = _msbuild_configuration(debug=debug)
    build_outdir = build_out_dir(arch, configuration)

    _build(
        ctx,
        env,
        arch=arch,
        configuration=configuration,
        vstudio_root=vstudio_root,
    )

    # Generate the config file
    if not ctx.run(
        f'inv -e generate-config --build-type="agent-py2py3" --output-file="{build_outdir}\\datadog.yaml"',
        warn=True,
        env=env,
    ):
        raise Exit("Could not generate test datadog.yaml file")

    # Run the tests
    if not ctx.run(f'dotnet test {build_outdir}\\CustomActions.Tests.dll', warn=True, env=env):
        raise Exit(code=1)

    if not ctx.run(f'dotnet test {build_outdir}\\WixSetup.Tests.dll', warn=True, env=env):
        raise Exit(code=1)


def validate_msi_createfolder_table(db):
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
    allowlist = ["APPLICATIONDATADIRECTORY", "checks.d", "run", "logs", "ProgramMenuDatadog"]

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
def validate_msi(_ctx, msi=None):
    with MsiClosing(msilib.OpenDatabase(msi, msilib.MSIDBOPEN_READONLY)) as db:
        validate_msi_createfolder_table(db)


@contextmanager
def MsiClosing(obj):
    """
    The MSI objects use Close() instead of close() so we can't use the built-in closing()
    """
    try:
        yield obj
    finally:
        obj.Close()
