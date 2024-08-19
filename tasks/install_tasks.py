import os
import platform
import shutil
import sys
import zipfile
from pathlib import Path

from invoke import Context, Exit, task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.go import download_go_dependencies
from tasks.libs.common.retry import run_command_with_retry
from tasks.libs.common.utils import bin_name, environ, gitlab_section

TOOL_LIST = [
    'github.com/frapposelli/wwhrd',
    'github.com/go-enry/go-license-detector/v4/cmd/license-detector',
    'github.com/golangci/golangci-lint/cmd/golangci-lint',
    'github.com/goware/modvendor',
    'github.com/stormcat24/protodep',
    'gotest.tools/gotestsum',
    'github.com/vektra/mockery/v2',
    'github.com/wadey/gocovmerge',
]

TOOL_LIST_PROTO = [
    'github.com/favadi/protoc-go-inject-tag',
    'github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway',
    'github.com/golang/protobuf/protoc-gen-go',
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
    with environ({'GO111MODULE': 'on'}):
        download_go_dependencies(ctx, paths=list(TOOLS.keys()))


@task
def install_tools(ctx: Context, max_retry: int = 3, custom_golangci_lint=True):
    """Install all Go tools for testing."""
    with gitlab_section("Installing Go tools", collapsed=True):
        with environ({'GO111MODULE': 'on'}):
            for path, tools in TOOLS.items():
                with ctx.cd(path):
                    for tool in tools:
                        run_command_with_retry(ctx, f"go install {tool}", max_retry=max_retry)
        if custom_golangci_lint:
            install_custom_golanci_lint(ctx)


def install_custom_golanci_lint(ctx):
    res = ctx.run("golangci-lint custom -v")
    if res.ok:
        gopath = os.getenv('GOPATH')
        gobin = os.getenv('GOBIN')

        golintci_binary = bin_name('golangci-lint')
        golintci_lint_backup_binary = bin_name('golangci-lint-backup')

        if gopath is None and gobin is None:
            print("Not able to install custom golangci-lint binary. golangci-lint won't work as expected")
            raise Exit(code=1)

        if gobin is not None and gopath is None:
            shutil.move(os.path.join(gobin, golintci_binary), os.path.join(gobin, golintci_lint_backup_binary))
            shutil.move(golintci_binary, os.path.join(gobin, golintci_binary))

        if gopath is not None:
            shutil.move(
                os.path.join(gopath, "bin", golintci_binary), os.path.join(gopath, "bin", golintci_lint_backup_binary)
            )
            shutil.move(golintci_binary, os.path.join(gopath, "bin", golintci_binary))

        print("Installed custom golangci-lint binary successfully")


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
def install_protoc(ctx, version="26.1"):
    """
    Installs the requested version of protoc in the specified folder (by default /usr/local/bin).
    Required generate the golang code based on .prod (inv generate-protobuf).
    """

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
