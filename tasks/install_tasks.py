import os
import platform
import sys
import zipfile
from pathlib import Path
from time import sleep

from invoke import Context, Exit, task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.go import download_go_dependencies
from tasks.libs.common.utils import environ, get_gobin, gitlab_section, link_or_copy

TOOL_LIST = [
    'github.com/bazelbuild/bazelisk',
    'github.com/frapposelli/wwhrd',
    'github.com/go-enry/go-license-detector/v4/cmd/license-detector',
    'github.com/golangci/golangci-lint/v2/cmd/golangci-lint',
    'github.com/goware/modvendor',
    'github.com/stormcat24/protodep',
    'gotest.tools/gotestsum',
    'github.com/vektra/mockery/v3',
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

        pending = [(path, tool) for path, tools in TOOLS.items() for tool in tools]
        for attempt in range(max_retry):
            last = attempt == max_retry - 1

            # Start all pending installs in parallel
            promises = []
            for path, tool in pending:
                with ctx.cd(path):
                    promise = ctx.run(f"go install {tool}", asynchronous=True, warn=not last, env=env)
                    promises.append((path, tool, promise))

            # Collect failures
            pending = []
            for path, tool, promise in promises:
                result = promise.join()
                if result.exited is None or result.exited > 0:
                    pending.append((path, tool))

            if pending and not last:
                wait = 10**attempt
                failed_names = [tool.rsplit('/', 1)[-1] for _, tool in pending]
                print(
                    f"[{attempt + 1} / {max_retry}] {len(pending)} tool(s) failed, retrying in {wait}s: {failed_names}"
                )
                sleep(wait)

        if pending:
            failed_list = '\n'.join(f"  {path}: {tool}" for path, tool in pending)
            raise Exit(f"Failed to install tools:\n{failed_list}", code=1)

        for bazelisk in Path(get_gobin(ctx)).glob('bazelisk*'):
            link_or_copy(bazelisk, bazelisk.with_stem(bazelisk.stem.replace('isk', '')))


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
