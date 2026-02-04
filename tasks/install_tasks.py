import os
import platform
import re
import shutil
import sys
import tarfile
import tempfile
import zipfile
from pathlib import Path

from invoke import Context, Exit, task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.go import download_go_dependencies
from tasks.libs.common.retry import run_command_with_retry
from tasks.libs.common.utils import environ, gitlab_section

TOOL_LIST = [
    'github.com/frapposelli/wwhrd',
    'github.com/go-enry/go-license-detector/v4/cmd/license-detector',
    'github.com/golangci/golangci-lint/v2/cmd/golangci-lint',
    'github.com/goware/modvendor',
    'github.com/stormcat24/protodep',
    'gotest.tools/gotestsum',
    'github.com/vektra/mockery/v2',
    'github.com/wadey/gocovmerge',
    'github.com/uber-go/gopatch',
    'github.com/aarzilli/whydeadcode',
]

TOOL_LIST_PROTO = [
    'github.com/favadi/protoc-go-inject-tag',
    'google.golang.org/protobuf/cmd/protoc-gen-go',
    'google.golang.org/grpc/cmd/protoc-gen-go-grpc',
    'github.com/golang/mock/mockgen',
    'github.com/planetscale/vtprotobuf/cmd/protoc-gen-go-vtproto',
    'github.com/tinylib/msgp',
]

TOOLS = {
    'internal/tools': TOOL_LIST,
    'internal/tools/proto': TOOL_LIST_PROTO,
}


@task
def download_tools(ctx):
    """Download all Go tools for testing."""
    print(color_message("This command is deprecated, please use `install-tools` instead", Color.ORANGE))
    with environ({'GO111MODULE': 'on'}):
        download_go_dependencies(ctx, paths=list(TOOLS.keys()))


@task
def install_tools(ctx: Context, max_retry: int = 3):
    """Install all Go tools for testing."""
    with gitlab_section("Installing Go tools", collapsed=True):
        env = {'GO111MODULE': 'on'}
        if os.getenv('DD_CC'):
            env['CC'] = os.getenv('DD_CC')
        if os.getenv('DD_CXX'):
            env['CXX'] = os.getenv('DD_CXX')
        with environ(env):
            for path, tools in TOOLS.items():
                with ctx.cd(path):
                    for tool in tools:
                        run_command_with_retry(ctx, f"go install {tool}", max_retry=max_retry)


@task
def install_shellcheck(ctx, version="0.8.0", destination="/usr/local/bin"):
    """
    Installs the requested version of shellcheck in the specified folder (by default /usr/local/bin).
    Required to run the shellcheck pre-commit hook.
    """

    if sys.platform == 'win32':
        print("shellcheck is not supported on Windows")
        raise Exit(code=1)
    if sys.platform.startswith('darwin'):
        platform = "darwin"
    if sys.platform.startswith('linux'):
        platform = "linux"

    ctx.run(
        f"wget -qO- \"https://github.com/koalaman/shellcheck/releases/download/v{version}/shellcheck-v{version}.{platform}.x86_64.tar.xz\" | tar -xJv -C /tmp"
    )
    ctx.run(f"cp \"/tmp/shellcheck-v{version}/shellcheck\" {destination}")
    ctx.run(f"rm -rf \"/tmp/shellcheck-v{version}\"")


@task
def install_rust_license_tool(ctx):
    """
    Install dd-rust-license-tool and cargo-deny for Rust license verification.
    Required to run the lint-rust-licenses task.
    """
    ctx.run("cargo install --git https://github.com/DataDog/rust-license-tool dd-rust-license-tool")
    ctx.run("cargo install cargo-deny --locked")


@task
def install_protoc(ctx, version=None):
    """
    Installs the requested version of protoc in the specified folder (by default /usr/local/bin).
    Required generate the golang code based on .prod (dda inv protobuf.generate).
    """
    if version is None:
        version_file = ".protoc-version"
        with open(version_file) as f:
            version = f.read().strip()
    if sys.platform == 'win32':
        print("protoc is not supported on Windows")
        raise Exit(code=1)
    if sys.platform.startswith('darwin'):
        platform_os = "osx"
    if sys.platform.startswith('linux'):
        platform_os = "linux"

    platform_arch = platform.machine().lower()
    if platform_arch == "amd64":
        platform_arch = "x86_64"
    elif platform_arch in {"aarch64", "arm64"}:
        platform_arch = "aarch_64"

    # Download the artifact thanks to the Github API class
    artifact_url = f"https://github.com/protocolbuffers/protobuf/releases/download/v{version}/protoc-{version}-{platform_os}-{platform_arch}.zip"
    zip_path = "/tmp"
    zip_name = "protoc"
    zip_file = os.path.join(zip_path, f"{zip_name}.zip")

    gh = GithubAPI(public_repo=True)
    # the download_from_url expect to have the path and the name of the file separated and without the extension
    gh.download_from_url(artifact_url, zip_path, zip_name)

    # Unzip it in the target destination
    destination = os.path.join(Path.home(), ".local")
    with zipfile.ZipFile(zip_file, "r") as zip_ref:
        zip_ref.extract('bin/protoc', path=destination)
    ctx.run(f"chmod +x {destination}/bin/protoc")
    ctx.run(f"rm {zip_file}")


@task
def install_devcontainer_cli(ctx):
    """
    Install the devcontainer CLI
    """
    ctx.run("npm install -g @devcontainers/cli")


# TODO: Test this
def _extract_onnxruntime_zip(archive_file, temp_dir, destination):
    """Extract onnxruntime from a zip file (Windows)."""
    with zipfile.ZipFile(archive_file, "r") as zip_ref:
        for member in zip_ref.namelist():
            if member.startswith("lib/") and member.endswith(".dll"):
                # DLLs go to bin on Windows
                bin_dir = os.path.join(destination, "bin")
                os.makedirs(bin_dir, exist_ok=True)
                zip_ref.extract(member, temp_dir)
                src = os.path.join(temp_dir, member)
                dst = os.path.join(bin_dir, os.path.basename(member))
                if os.path.exists(src):
                    shutil.move(src, dst)
            elif member.startswith("lib/") and member.endswith(".lib"):
                # .lib files go to lib
                zip_ref.extract(member, destination)
            elif member.startswith("include/onnxruntime/"):
                zip_ref.extract(member, destination)


def _extract_onnxruntime_tgz(archive_file, tmp_dir, destination):
    """Extract onnxruntime from a tgz file (Linux/macOS)."""
    re_lib = re.compile(r"^.*/lib/(.*(\.so|\.dylib)(\.\d+)?)$")
    re_include = re.compile(r"^.*/include/(.*\.h)$")
    to_move_files = []
    with tarfile.open(archive_file, "r:gz") as tar_ref:
        for member in tar_ref.getmembers():
            if match := re_lib.match(member.name):
                tar_ref.extract(member, tmp_dir)
                to_move_files.append((tmp_dir + '/' + member.name, destination + '/lib/' + match.group(1)))
            elif match := re_include.match(member.name):
                tar_ref.extract(member, tmp_dir)
                to_move_files.append((tmp_dir + '/' + member.name, destination + '/include/' + match.group(1)))

    for src, dst in to_move_files:
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        if not os.path.exists(dst):
            shutil.move(src, dst)


# TODO A: Dev is cleaned when we agent.clean, do it in .cache/onnxruntime?
@task
def install_onnxruntime(ctx, version="1.23.2", destination=None):
    """
    Installs the requested version of onnxruntime in the dev directory.
    Required for building the agent with onnxruntime support in non-omnibus builds.
    """
    from tasks.libs.common.utils import get_repo_root

    if version is None:
        version = "1.23.2"

    # Determine platform
    if sys.platform == 'win32':
        platform_os = "win"
        platform_arch = "x64" if platform.machine().lower() in {"amd64", "x86_64"} else "x86"
        file_ext = ".zip"
    elif sys.platform.startswith('darwin'):
        platform_os = "osx"
        platform_arch = "arm64" if platform.machine().lower() in {"arm64", "aarch64"} else "x64"
        file_ext = ".tgz"
    elif sys.platform.startswith('linux'):
        platform_os = "linux"
        platform_arch = "aarch64" if platform.machine().lower() in {"aarch64", "arm64"} else "x64"
        file_ext = ".tgz"
    else:
        print(f"Unsupported platform: {sys.platform}")
        raise Exit(code=1)

    # Determine destination (dev directory in repo root)
    if destination is None:
        repo_root = get_repo_root()
        destination = os.path.join(repo_root, "dev")
    else:
        destination = os.path.abspath(destination)

    # Create destination directories
    lib_dir = os.path.join(destination, "lib")
    include_dir = os.path.join(destination, "include", "onnxruntime")
    os.makedirs(lib_dir, exist_ok=True)
    os.makedirs(include_dir, exist_ok=True)

    # Download URL
    artifact_url = f"https://github.com/microsoft/onnxruntime/releases/download/v{version}/onnxruntime-{platform_os}-{platform_arch}-{version}{file_ext}"

    print(color_message(f"Downloading onnxruntime {version} for {platform_os}-{platform_arch}...", Color.BLUE))
    print(color_message(f"URL: {artifact_url}", Color.BLUE))

    # Download the artifact
    temp_dir = tempfile.mkdtemp()
    archive_file = os.path.join(temp_dir, f"onnxruntime{file_ext}")
    try:
        # Download the file
        if file_ext == ".zip":
            gh = GithubAPI(public_repo=True)
            gh.download_from_url(artifact_url, temp_dir, "onnxruntime")
            # GithubAPI might download with a different name, find the zip file
            zip_files = [f for f in os.listdir(temp_dir) if f.endswith('.zip')]
            if zip_files:
                archive_file = os.path.join(temp_dir, zip_files[0])
            else:
                archive_file = os.path.join(temp_dir, "onnxruntime.zip")
        else:
            # Download .tgz files using requests
            import requests
            response = requests.get(artifact_url, stream=True, timeout=10)
            response.raise_for_status()
            with open(archive_file, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    f.write(chunk)

        print(color_message(f"Extracting onnxruntime to {destination}...", Color.BLUE))

        # Extract files
        if file_ext == ".zip":
            _extract_onnxruntime_zip(archive_file, temp_dir, destination)
        else:
            _extract_onnxruntime_tgz(archive_file, temp_dir, destination)

        print(color_message(f"onnxruntime {version} installed successfully to {destination}", Color.GREEN))
    finally:
        # Cleanup
        shutil.rmtree(temp_dir, ignore_errors=True)
