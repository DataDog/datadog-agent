import glob
import os
import shutil
import tempfile
import zipfile
from pathlib import Path, PurePath

from gitlab.v4.objects import Project
from invoke import task

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.utils import download_to_tempfile


class PathStore:
    path: Path

    def __init__(self, path: str | Path):
        self.path = Path(path)

    def add(self, src: Path, dst: Path) -> Path:
        dst.mkdir(parents=True, exist_ok=True)
        shutil.copytree(src, dst, dirs_exist_ok=True)
        return dst

    def get(self, path: Path) -> Path | None:
        if path.exists():
            return path
        return None


class SymbolStore(PathStore):
    def __init__(self, path: str | Path):
        super().__init__(path)

    def add(self, version: str, path: str | Path) -> Path:
        dst = Path(self.path, version, 'symbols')
        return super().add(path, dst)

    def get(self, version: str) -> Path | None:
        p = Path(self.path, version, 'symbols')
        return super().get(p)


class ArtifactStore(PathStore):
    def __iter__(self):
        return iter(self.path.glob('**/*'))

    def add(self, project: str, job: str, path: str | Path) -> Path:
        k = self.__key(project, job)
        dst = Path(k, 'artifacts')
        return super().add(path, dst)

    def get(self, project: str, job: str) -> Path | None:
        k = self.__key(project, job)
        k = Path(k, 'artifacts')
        return super().get(k)

    def add_version(self, project: str, job: str, version: str) -> None:
        k = self.__key(project, job)
        if not k.exists():
            raise ValueError(f"Job {job} not found in artifact store")
        dst = Path(k, 'version.txt')
        dst.write_text(version, encoding='utf-8')
        return dst

    def get_version(self, project: str, job: str) -> str | None:
        k = self.__key(project, job)
        if not k.exists():
            return None
        version = Path(k, 'version.txt')
        if not version.exists():
            return None
        return version.read_text(encoding='utf-8')

    def __key(self, project: str, job: str) -> Path:
        return Path(self.path, project, job)


def add_gitlab_job_artifacts_to_artifact_store(artifact_store: ArtifactStore, project: Project, job_id: str) -> Path:
    with tempfile.TemporaryDirectory() as temp_dir:
        download_job_artifacts(project, job_id, temp_dir)
        project_id = "datadog-agent"  # TODO: get from project
        job_id = str(job_id)
        return artifact_store.add(project_id, job_id, temp_dir)


class CrashAnalyzerCLI:
    env: Path

    active_dump: Path
    symbol_store: SymbolStore
    active_symbol: Path
    artifact_store: ArtifactStore
    active_project: Project

    def __init__(self, env=None):
        if env is None:
            env = Path(tempfile.mkdtemp(prefix='crash-analyzer-'))
        self.env = env

        self.active_dump = None

        self.symbol_store = SymbolStore(Path(env, 'symbols'))
        self.active_symbol = None

        self.artifact_store = ArtifactStore(Path(env, 'artifacts'))

    def select_dump(self, path: str | Path):
        path = Path(path)
        self.active_dump = path

    def select_symbol(self, path: str | Path):
        assert path in self.symbol_files
        self.active_symbol = path

    def select_project(self, project: Project):
        self.active_project = project


def get_cli():
    env = Path.home() / '.agent-crash-analyzer'
    cli = CrashAnalyzerCLI(env=env)
    print(f"Using environment: {cli.env}")
    return cli


@task(
    help={
        "job_id": "The job ID to download the dump from",
    },
)
def debug_job_dump(ctx, job_id):
    cli = get_cli()
    cli.select_project(get_gitlab_repo())

    # select dump file
    package_artifacts = get_or_fetch_artifacts(cli.artifact_store, cli.active_project, job_id)
    print("Dump files:")
    for dmp_file in find_dmp_files(package_artifacts):
        print('\t', Path(dmp_file).resolve())
    dump_file = input("Select a dump file to analyze: ")

    # select symbol file
    syms = get_symbols_for_job_id(cli, job_id)
    print("Symbols:")
    for symbol_file in find_symbol_files(syms):
        print('\t', Path(symbol_file).resolve())
    symbol_file = input("Select a symbol file to use: ")

    # launch windbg and delve
    windbg_cmd = f'cmd.exe /c start "" "{dump_file}"'
    print(f"Running command: {windbg_cmd}")
    dlv_cmd = f'dlv.exe core "{symbol_file}" "{dump_file}"'
    print(f"Running command: {dlv_cmd}")
    os.system(windbg_cmd)
    os.system(dlv_cmd)


@task(
    help={
        "job_id": "The job ID to download the dump from",
        "with_symbols": "Whether to download debug symbols",
    },
)
def get_job_dump(ctx, job_id, with_symbols=False):
    """
    Download a dump from a job and save it to the output directory.
    """
    cli = get_cli()
    cli.select_project(get_gitlab_repo())

    package_artifacts = get_or_fetch_artifacts(cli.artifact_store, cli.active_project, job_id)
    dmp_files = find_dmp_files(package_artifacts)
    if not dmp_files:
        print("No dump files found")
        return
    print("Dump files:")
    for dmp_file in dmp_files:
        print('\t', dmp_file)

    if with_symbols:
        syms = get_symbols_for_job_id(cli, job_id)
        print("Symbols:")
        for symbol_file in find_symbol_files(syms):
            print('\t', Path(symbol_file).resolve())


@task
def get_debug_symbols(ctx, job_id=None, version=None):
    cli = get_cli()
    if version:
        with tempfile.TemporaryDirectory() as tmp_dir:
            get_debug_symbols_for_version(version, tmp_dir)
            syms = cli.symbol_store.add(version, tmp_dir)
    elif job_id:
        cli.select_project(get_gitlab_repo())
        syms = get_symbols_for_job_id(cli, job_id)

    print(f"Symbols for {version} in {syms}")


def get_symbols_for_job_id(cli: CrashAnalyzerCLI, job_id: str) -> Path:
    version = cli.artifact_store.get_version("datadog-agent", job_id)
    if version:
        syms = cli.symbol_store.get(version)
        if syms:
            return syms
    # Need to get the symbols from the package build job in the pipeline
    package_job_id = get_package_job_id(cli.active_project, job_id)
    package_artifacts = get_or_fetch_artifacts(cli.artifact_store, cli.active_project, package_job_id)
    for debug_zip in find_debug_zip(package_artifacts):
        debug_zip = Path(debug_zip)
        version = debug_zip.name.removesuffix('.debug.zip')
        syms = cli.symbol_store.get(version)
        if not syms:
            with tempfile.TemporaryDirectory() as tmp_dir:
                extract_agent_symbols(debug_zip, tmp_dir)
                syms = cli.symbol_store.add(version, tmp_dir)
    # add a version ref so we can look it up faster next time
    cli.artifact_store.add_version("datadog-agent", package_job_id, version)
    cli.artifact_store.add_version("datadog-agent", job_id, version)
    return syms


def get_or_fetch_artifacts(artifact_store: ArtifactStore, project: Project, job_id: str) -> Path:
    project_id = "datadog-agent"  # TODO: get from project
    artifacts = artifact_store.get(project_id, job_id)
    if not artifacts:
        artifacts = add_gitlab_job_artifacts_to_artifact_store(artifact_store, project, job_id)
    return artifacts


def get_debug_symbols_for_version(version: str, output_dir=None) -> None:
    url = get_debug_symbol_url_for_version(version)
    print(f"Downloading symbols for {version} from {url}")
    with download_to_tempfile(url) as zip_path:
        extract_agent_symbols(zip_path, output_dir)


def get_debug_symbols_for_job_pipeline(job_id: str, output_dir=None) -> None:
    package_job_id = get_package_job_id(job_id)
    print(f"Downloading debug symbols from package job {package_job_id}")
    # TODO: gitlab API doesn't let us download just one artifact
    #       so we have to get them all, and they are big :(
    #       we could start uploading the .debug.zip to mstesting bucket
    package_out = Path(output_dir, f'{package_job_id}-artifacts')
    download_job_artifacts(package_job_id, package_out)
    debug_zip = find_debug_zip(package_out)
    extract_agent_symbols(debug_zip, output_dir)


def extract_agent_symbols(zip_path: str, output_dir: str) -> None:
    with zipfile.ZipFile(zip_path, "r") as zip_ref:
        for info in zip_ref.infolist():
            if info.filename.endswith('.exe.debug'):
                info.filename = PurePath(info.filename).name
                zip_ref.extract(info, output_dir)


def get_debug_symbol_url_for_version(version: str) -> str:
    if 'rc' in version:
        base = 'https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/ddagent-cli-'
    else:
        base = 'https://s3.amazonaws.com/ddagent-windows-stable/ddagent-cli-'
    url = f'{base}{version}.debug.zip'
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


def find_dmp_files(output_dir: str) -> list[str]:
    return list(glob.glob(f"{output_dir}/**/*.dmp", recursive=True))


def find_debug_zip(output_dir: str) -> str:
    return list(glob.glob(f"{output_dir}/**/*.debug.zip", recursive=True))


def find_symbol_files(output_dir: str) -> list[str]:
    return list(glob.glob(f"{output_dir}/**/*.exe.debug", recursive=True))


def get_package_job_id(project: Project, job_id: str, package_job_name=None) -> str | None:
    """
    Get the package job ID for the pipeline of the given job.
    """
    if package_job_name is None:
        package_job_name = "windows_msi_and_bosh_zip_x64-a7"

    job = project.jobs.get(job_id)
    pipeline_id = str(job.pipeline["id"])
    pipeline = project.pipelines.get(pipeline_id)
    jobs = pipeline.jobs.list(iterator=True, per_page=50, scope='success')
    for job in jobs:
        if job.name == package_job_name:
            return str(job.id)
