import glob
import os
import tempfile
import zipfile
from pathlib import Path, PurePath

from invoke import task

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.utils import download_to_tempfile


@task(
    help={
        "job_id": "The job ID to download the dump from",
        "output_dir": "The directory to save the dump to",
    },
)
def debug_job_dump(ctx, job_id=None, output_dir=None):
    if output_dir is None:
        output_dir = tempfile.mkdtemp()

    if job_id:
        get_job_dump(ctx, job_id, with_symbols=True, output_dir=output_dir)
    else:
        print("Dump files:")
        for dmp_file in find_dmp_files(output_dir):
            print('\t', Path(dmp_file).resolve())
        print("Symbols:")
        for symbol_file in find_symbol_files(output_dir):
            print('\t', Path(symbol_file).resolve())

    # prompt user to select a dump file
    dump_file = input("Select a dump file to analyze: ")
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
        "output_dir": "The directory to save the dump to",
    },
)
def get_job_dump(ctx, job_id, with_symbols=False, output_dir=None):
    """
    Download a dump from a job and save it to the output directory.
    """
    if output_dir is None:
        output_dir = tempfile.mkdtemp()

    artifacts_dir = Path(output_dir, 'artifacts')

    download_job_artifacts(job_id, artifacts_dir)
    dmp_files = find_dmp_files(artifacts_dir)
    if not dmp_files:
        print("No dump files found")
        return
    print("Dump files:")
    for dmp_file in dmp_files:
        print('\t', dmp_file)

    if with_symbols:
        get_debug_symbols(ctx, job_id=job_id, output_dir=output_dir)
        print("Symbols:")
        for symbol_file in find_symbol_files(output_dir):
            print('\t', Path(symbol_file).resolve())


@task
def get_debug_symbols(ctx, job_id=None, version=None, output_dir=None):
    if output_dir is None:
        output_dir = tempfile.mkdtemp()

    symbol_out = Path(output_dir, 'symbols')

    if version:
        get_debug_symbols_for_version(version, symbol_out)
    elif job_id:
        get_debug_symbols_for_job_pipeline(job_id, symbol_out)

    print(f"Downloaded debug symbols to {symbol_out}")


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


def download_job_artifacts(job_id: str, output_dir: str) -> None:
    """
    Download the artifacts for a job to the output directory.
    """
    gitlab = get_gitlab_repo()
    job = gitlab.jobs.get(job_id)
    print(f"Downloading artifacts for job {job.name} to {output_dir}")
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
    return glob.glob(f"{output_dir}/**/*.debug.zip", recursive=True)[0]


def find_symbol_files(output_dir: str) -> list[str]:
    return list(glob.glob(f"{output_dir}/**/*.exe.debug", recursive=True))


def get_package_job_id(job_id: str, package_job_name=None) -> str | None:
    """
    Get the package job ID for the pipeline of the given job.
    """
    if package_job_name is None:
        package_job_name = "windows_msi_and_bosh_zip_x64-a7"

    gitlab = get_gitlab_repo()
    job = gitlab.jobs.get(job_id)
    pipeline_id = job.pipeline["id"]
    pipeline = gitlab.pipelines.get(pipeline_id)
    jobs = pipeline.jobs.list(iterator=True)
    for job in jobs:
        if job.name == package_job_name:
            return job.id
