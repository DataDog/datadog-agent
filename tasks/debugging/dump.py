import glob
import os
import shutil
import sys
import tempfile
import zipfile
from collections.abc import Sequence
from pathlib import Path, PurePath

from gitlab.v4.objects import Project
from invoke.tasks import task

from tasks.debugging.delve import Delve
from tasks.debugging.gitlab_artifacts import Artifacts, ArtifactStore
from tasks.debugging.symbols import SymbolStore
from tasks.debugging.windbg import Windbg
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.utils import download_to_tempfile

LINUX_PLATFORMS = ['suse', 'redhat', 'debian', 'ubuntu']
PLATFORM_CHOICES = ['windows'] + LINUX_PLATFORMS
ARCH_CHOICES = ['x86_64', 'arm64']


class CrashAnalyzer:
    env: Path

    target_platform: str
    target_arch: str

    active_dump: Path | None
    symbol_store: SymbolStore
    active_symbol: Path | None
    artifact_store: ArtifactStore
    active_project: Project | None

    def __init__(self, env=None, platform=None, arch=None):
        if env is None:
            env = Path(tempfile.mkdtemp(prefix='crash-analyzer-'))
        self.env = env

        self.__init_platform(platform, arch)

        self.active_dump = None

        self.symbol_store = SymbolStore(Path(env, 'symbols'))
        self.active_symbol = None

        self.artifact_store = ArtifactStore(Path(env, 'artifacts'))

    def __init_platform(self, platform=None, arch=None):
        if platform:
            self.target_platform = platform
        else:
            if sys.platform == 'win32':
                self.target_platform = 'windows'
            else:
                self.target_platform = 'debian'
        if arch:
            self.target_arch = arch
        else:
            self.target_arch = 'x86_64'

    def select_dump(self, path: str | Path):
        self.active_dump = Path(path)

    def select_symbol(self, path: str | Path):
        self.active_symbol = Path(path)

    def select_project(self, project: Project):
        self.active_project = project


class CrashAnalyzerCli:
    ca: CrashAnalyzer

    def __init__(self, crash_analyzer):
        self.ca = crash_analyzer

    def prompt_select_dump(self, choices: Sequence[Path | str]):
        print("Dump files:")
        for choice in choices:
            print('\t', choice)
        if len(choices) > 1:
            choice = ""
            while not choice:
                choice = input("Select a dump file: ")
        self.ca.select_dump(choice)

    def prompt_select_symbol_file(self, choices: Sequence[Path | str]):
        print("Symbol files:")
        for choice in choices:
            print('\t', choice)
        if len(choices) > 1:
            choice = ""
            while not choice:
                choice = input("Select a symbol file: ")
        self.ca.select_symbol(choice)


def get_crash_analyzer(platform=None, arch=None):
    # TODO: configuration file would be nice
    env = Path.home() / '.agent-crash-analyzer'
    ca = CrashAnalyzer(env=env, platform=platform, arch=arch)
    print(f"Using environment: {ca.env}")
    return ca


@task(
    help={
        "job_id": "The job ID to download the dump from",
        "platform": f"The target platform ({', '.join(PLATFORM_CHOICES)})",
        "arch": f"The target architecture ({', '.join(ARCH_CHOICES)})",
    },
)
def debug_job_dump(ctx, job_id: str, platform=None, arch=None):
    """
    Launch delve or windbg to debug a dump from a job's artifacts.

    Downloads debug symbols from the pipeline's package build job.
    """
    ca = get_crash_analyzer(platform=platform, arch=arch)
    ca.select_project(get_gitlab_repo())
    assert ca.active_project
    cli = CrashAnalyzerCli(ca)

    # select dump file
    package_artifacts = get_or_fetch_artifacts(ca.artifact_store, ca.active_project, job_id)
    path = package_artifacts.get()
    if not path:
        print("No artifacts found")
        return
    dmp_files = find_dmp_files(path)
    if not dmp_files:
        print("No dump files found")
        return
    cli.prompt_select_dump(dmp_files)
    assert ca.active_dump

    # select symbol file
    _, syms = get_symbols_for_job_id(ca, job_id)
    if not syms:
        print("No symbols found")
        return
    syms = find_symbol_files_for_platform(ca.target_platform, syms)
    if not syms:
        print("No symbols found")
        return
    cli.prompt_select_symbol_file(syms)
    assert ca.active_symbol

    # launch windbg and delve
    if sys.platform == 'win32':
        windbg = Windbg()
        windbg.open_dump(ca.active_dump)

    dlv = Delve()
    dlv.core(ca.active_symbol, ca.active_dump)


@task(
    help={
        "job_id": "The job ID to download the dump from",
        "with_symbols": "Whether to download debug symbols from the package build job",
        "platform": f"The target platform ({', '.join(PLATFORM_CHOICES)})",
        "arch": f"The target architecture ({', '.join(ARCH_CHOICES)})",
    },
)
def get_job_dump(ctx, job_id: str, with_symbols=False, platform=None, arch=None):
    """
    Download a dump from a job and save it to the output directory.

    Optionally download debug symbols from the pipeline's package build job.
    """
    ca = get_crash_analyzer(platform=platform, arch=arch)
    ca.select_project(get_gitlab_repo())
    assert ca.active_project

    package_artifacts = get_or_fetch_artifacts(ca.artifact_store, ca.active_project, job_id)
    path = package_artifacts.get()
    if not path:
        print("No artifacts found")
        return
    dmp_files = find_dmp_files(path)
    if not dmp_files:
        print("No dump files found")
        return
    print("Dump files:")
    for dmp_file in dmp_files:
        print('\t', dmp_file)

    if with_symbols:
        _, syms = get_symbols_for_job_id(ca, job_id)
        if not syms:
            print("No symbols found")
            return
        print("Symbols:")
        for symbol_file in find_symbol_files_for_platform(ca.target_platform, syms):
            print('\t', Path(symbol_file).resolve())


@task(
    help={
        "job_id": "Any job ID from the pipeline to download the symbols from",
        "version": "The released Agent version to download symbols for (e.g. 7.57.0)",
        "platform": f"The target platform ({', '.join(PLATFORM_CHOICES)})",
        "arch": f"The target architecture ({', '.join(ARCH_CHOICES)})",
    }
)
def get_debug_symbols(
    ctx, job_id: str | None = None, version: str | None = None, platform: str | None = None, arch: str | None = None
):
    """
    Download debug symbols for a specific Agent version or pipeline
    """
    ca = get_crash_analyzer(platform=platform, arch=arch)

    if version:
        syms = get_or_fetch_symbols_for_version(ca, version)
    elif job_id:
        ca.select_project(get_gitlab_repo())
        version, syms = get_symbols_for_job_id(ca, job_id)

    print(f"Symbols for {version} in {syms}")


def add_gitlab_job_artifacts_to_artifact_store(
    artifact_store: ArtifactStore, project: Project, job_id: str
) -> Artifacts:
    with tempfile.TemporaryDirectory() as temp_dir:
        download_job_artifacts(project, job_id, temp_dir)
        project_id = project.name
        job_id = str(job_id)
        return artifact_store.add(project_id, job_id, temp_dir)


def get_symbols_for_job_id(ca: CrashAnalyzer, job_id: str) -> tuple[str | None, Path | None]:
    if not ca.active_project:
        raise ValueError("No active project set")
    project_id = ca.active_project.name
    # check if we already have the symbols for this job
    artifact = ca.artifact_store.get(project_id, job_id)
    syms = None
    if artifact and artifact.version:
        syms = ca.symbol_store.get(artifact.version, ca.target_platform, ca.target_arch)
        if syms:
            return artifact.version, syms
    # Need to get the symbols from the package build job in the pipeline
    # TODO: Linux platforms could get symbols from the testing repos
    package_job_id = get_package_job_id(ca.active_project, job_id, ca.target_platform, ca.target_arch)
    if not package_job_id:
        raise Exception(f"Could not find package job for job {job_id}")
    package_artifacts = get_or_fetch_artifacts(ca.artifact_store, ca.active_project, package_job_id)
    path = package_artifacts.get()
    if not path:
        raise FileNotFoundError(f"No artifacts found for job {package_job_id}")
    archives = find_platform_debug_artifacts(ca.target_platform, path)
    # TODO: hacky way to get the version from the archive name, is there a better way?
    #       the setup-agent-version job stores the version in a private bucket.
    version = Path(archives[0]).name
    for s in ['.debug.zip', '.tar.xz', '-amd64', '-x86_64', '-arm64', '-1']:
        version = version.removesuffix(s)
    for p in ['datadog-agent-dbg-', 'datadog-agent-']:
        version = version.removeprefix(p)
    # add a version ref so we can look it up faster next time
    package_artifacts.version = version
    if not artifact:
        artifact = ca.artifact_store.add(project_id, job_id)
    artifact.version = version
    syms = ca.symbol_store.get(version, ca.target_platform, ca.target_arch)
    if not syms:
        # add the symbols to the symbol store
        for path in archives:
            with tempfile.TemporaryDirectory() as tmp_dir:
                if ca.target_platform == 'windows':
                    _windows_extract_agent_symbols(path, tmp_dir)
                elif ca.target_platform in LINUX_PLATFORMS:
                    _linux_extract_agent_symbols(path, tmp_dir)
                else:
                    raise NotImplementedError(f"Unsupported platform {ca.target_platform}")
                syms = ca.symbol_store.add(version, ca.target_platform, ca.target_arch, tmp_dir)

    return version, syms


def get_or_fetch_artifacts(artifact_store: ArtifactStore, project: Project, job_id: str) -> Artifacts:
    project_id = project.name
    artifacts = artifact_store.get(project_id, job_id)
    if not artifacts or not artifacts.get():
        artifacts = add_gitlab_job_artifacts_to_artifact_store(artifact_store, project, job_id)
    return artifacts


def get_or_fetch_symbols_for_version(ca: CrashAnalyzer, version: str) -> Path:
    syms = ca.symbol_store.get(version, ca.target_platform, ca.target_arch)
    if not syms:
        with tempfile.TemporaryDirectory() as tmp_dir:
            get_debug_symbols_for_version(version, ca.target_platform, ca.target_arch, tmp_dir)
            syms = ca.symbol_store.add(version, ca.target_platform, ca.target_arch, tmp_dir)
    return syms


def get_debug_symbols_for_version(version: str, platform: str, arch: str, output_dir: Path | str) -> None:
    if platform == 'linux':
        raise ValueError("Must specify a distribution instead of 'linux'")
    if platform == 'windows':
        _windows_get_debug_symbols_for_version(version, output_dir)
    elif platform == 'suse':
        _suse_get_debug_symbols_for_version(version, arch, output_dir)
    elif platform == 'redhat':
        _yum_get_debug_symbols_for_version(version, arch, output_dir)
    elif platform in ['debian', 'ubuntu']:
        _apt_get_debug_symbols_for_version(version, arch, output_dir)
    else:
        raise NotImplementedError(f"Unsupported platform {platform}")


def is_rc_version(version: str) -> bool:
    return 'rc' in version


def _apt_get_debug_symbols_for_version(version: str, arch: str, output_dir: Path | str) -> None:
    if is_rc_version(version):
        base = 'https://s3.amazonaws.com/apt.datad0g.com/pool/d/da/'
    else:
        base = 'https://s3.amazonaws.com/apt.datadoghq.com/pool/d/da/'
    if arch == 'x86_64':
        arch = 'amd64'
    url = f'{base}datadog-agent-dbg_{version}-1_{arch}.deb'
    print(f"Downloading symbols for {version} from {url}")
    with download_to_tempfile(url) as deb_path:
        _deb_extract_agent_symbols(deb_path, output_dir)


def _deb_extract_agent_symbols(deb_path: Path | str, output_dir: Path | str) -> None:
    assert shutil.which('dpkg-deb'), "dpkg-deb is required to extract symbols from DEBs"
    with tempfile.TemporaryDirectory() as tmp_dir:
        os.system(f'dpkg-deb --raw-extract {deb_path} {tmp_dir}')
        # example: opt/datadog-agent/.debug/opt/datadog-agent/bin/agent/agent.dbg
        debug_root = Path(tmp_dir) / 'opt/datadog-agent/.debug'
        shutil.copytree(debug_root, output_dir, dirs_exist_ok=True)


def _suse_get_debug_symbols_for_version(version: str, arch: str, output_dir: Path | str) -> None:
    if is_rc_version(version):
        base = 'https://s3.amazonaws.com/yum.datad0g.com/suse/beta/'
    else:
        base = 'https://s3.amazonaws.com/yum.datadoghq.com/suse/stable/'
    return _yum_get_debug_symbols_for_version(version, arch, output_dir, baseurl=base)


def _yum_get_debug_symbols_for_version(
    version: str, arch: str, output_dir: Path | str, baseurl: str | None = None
) -> None:
    if baseurl is None:
        if is_rc_version(version):
            baseurl = 'https://s3.amazonaws.com/yum.datad0g.com/beta/'
        else:
            baseurl = 'https://s3.amazonaws.com/yum.datadoghq.com/stable/'
    major_version = version.split('.')[0]
    if arch == 'arm64':
        arch = 'aarch64'
    url = f'{baseurl}{major_version}/{arch}/datadog-agent-dbg-{version}-1.{arch}.rpm'
    print(f"Downloading symbols for {version} from {url}")
    with download_to_tempfile(url) as rpm_path:
        _rpm_extract_agent_symbols(rpm_path, output_dir)


def _rpm_extract_agent_symbols(rpm_path: Path | str, output_dir: Path | str) -> None:
    assert shutil.which('rpm2cpio'), "rpm2cpio is required to extract symbols from RPMs"
    assert shutil.which('cpio'), "cpio is required to extract symbols from RPMs"
    with tempfile.TemporaryDirectory() as tmp_dir:
        # example: opt/datadog-agent/.debug/opt/datadog-agent/bin/agent/agent.dbg
        debug_root = 'opt/datadog-agent/.debug'
        os.system(f'rpm2cpio {rpm_path} | cpio -idm -D {tmp_dir}')
        shutil.copytree(f"{tmp_dir}/{debug_root}", output_dir, dirs_exist_ok=True)


def _linux_extract_agent_symbols(archive_path: Path | str, output_dir: Path | str) -> None:
    with tempfile.TemporaryDirectory() as tmp_dir:
        shutil.unpack_archive(archive_path, tmp_dir)
        # example: opt/datadog-agent/.debug/opt/datadog-agent/bin/agent/agent.dbg
        debug_root = Path(tmp_dir) / 'opt/datadog-agent/.debug'
        shutil.copytree(debug_root, output_dir, dirs_exist_ok=True)


def _windows_extract_agent_symbols(zip_path: Path | str, output_dir: Path | str) -> None:
    with zipfile.ZipFile(zip_path, "r") as zip_ref:
        for info in zip_ref.infolist():
            if info.filename.endswith('.exe.debug'):
                info.filename = PurePath(info.filename).name
                zip_ref.extract(info, output_dir)


def _windows_get_debug_symbols_for_version(version: str, output_dir: Path | str) -> str:
    if is_rc_version(version):
        base = 'https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/ddagent-cli-'
    else:
        base = 'https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-'
    url = f'{base}{version}.debug.zip'
    print(f"Downloading symbols for {version} from {url}")
    with download_to_tempfile(url) as zip_path:
        _windows_extract_agent_symbols(zip_path, output_dir)
    return url


def download_job_artifacts(project: Project, job_id: str, output_dir: str) -> None:
    """
    Download the artifacts for a job to the output directory.
    """
    job = project.jobs.get(job_id)
    print(f"Downloading artifacts for job {job.name}")
    fd, tmp_path = tempfile.mkstemp()
    try:
        with os.fdopen(fd, "wb") as f:
            # fd will be closed by context manager, so we no longer need it
            fd = None
            job.artifacts(streamed=True, action=f.write)
        with zipfile.ZipFile(tmp_path, "r") as zip_ref:
            zip_ref.extractall(output_dir)
    finally:
        if fd is not None:
            os.close(fd)
        if os.path.exists(tmp_path):
            os.remove(tmp_path)


def find_dmp_files(path: Path | str) -> list[str]:
    if not os.path.exists(path):
        raise FileNotFoundError(f"Path {path} does not exist")
    return list(glob.glob(f"{path}/**/*.dmp", recursive=True))


def find_symbol_files_for_platform(platform: str, path: Path | str) -> list[str]:
    if platform == 'windows':
        return list(glob.glob(f"{path}/**/*.exe.debug", recursive=True))
    elif platform in LINUX_PLATFORMS:
        return list(glob.glob(f"{path}/**/*.dbg", recursive=True))
    else:
        raise NotImplementedError(f"Unsupported platform {platform}")


def find_platform_debug_artifacts(platform: str, path: Path | str) -> list[str]:
    if not os.path.exists(path):
        raise FileNotFoundError(f"Path {path} does not exist")
    if platform == 'windows':
        return list(glob.glob(f"{path}/**/*.debug.zip", recursive=True))
    elif platform in LINUX_PLATFORMS:
        return list(glob.glob(f"{path}/**/datadog-agent-dbg-*.tar.xz", recursive=True))
    else:
        raise NotImplementedError(f"Unsupported platform {platform}")


def get_package_job_id(project: Project, job_id: str, platform: str, arch: str) -> str | None:
    """
    Get the package job ID for the pipeline of the given job.
    """
    if platform == 'windows':
        package_job_name = "windows_msi_and_bosh_zip_x64-a7"
    elif platform in LINUX_PLATFORMS:
        if arch == 'x86_64':
            package_job_name = "datadog-agent-7-x64"
        elif arch == 'arm64':
            package_job_name = "datadog-agent-7-arm64"
        else:
            raise NotImplementedError(f"Unsupported arch {arch}")
    else:
        raise NotImplementedError(f"Unsupported platform {platform}")

    # TODO: there may be a name lookup option soon https://gitlab.com/gitlab-org/gitlab/-/issues/22027
    job = project.jobs.get(job_id)
    pipeline_id = str(job.pipeline["id"])
    pipeline = project.pipelines.get(pipeline_id)
    jobs = pipeline.jobs.list(iterator=True, per_page=50, scope='success')
    for job in jobs:
        if job.name == package_job_name:
            return str(job.id)
    return None
