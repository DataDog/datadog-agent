import os
import platform
import re
import sys
import tempfile
import zipfile
from dataclasses import dataclass
from pathlib import Path

from invoke import Context, Exit, task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.go import download_go_dependencies
from tasks.libs.common.retry import run_command_with_retry
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


@dataclass
class PrebuiltTool:
    """Describes how to download a pre-built binary from GitHub Releases."""

    repo: str  # GitHub repo (e.g. 'bazelbuild/bazelisk')
    binary_name: str  # Name of the binary to place in GOBIN
    # Asset filename pattern. Placeholders: {version} (without 'v' prefix), {os}, {arch}
    asset_pattern: str
    is_archive: bool = True  # If False, asset is a raw binary (not tar.gz)
    # Path to binary inside the archive (empty = binary at archive root)
    archive_binary_path: str = ""
    # go.mod module path used to look up the pinned version
    go_mod_module: str = ""
    # Override arch names when they differ from Go conventions (amd64/arm64)
    arch_map: dict[str, str] | None = None


# Tools that should be downloaded as pre-built binaries instead of `go install`.
# These are the slowest to compile from source and all provide darwin/arm64 binaries.
PREBUILT_TOOLS: dict[str, PrebuiltTool] = {
    'github.com/bazelbuild/bazelisk': PrebuiltTool(
        repo='bazelbuild/bazelisk',
        binary_name='bazelisk',
        asset_pattern='bazelisk-{os}-{arch}',
        is_archive=False,
        go_mod_module='github.com/bazelbuild/bazelisk',
    ),
    'github.com/golangci/golangci-lint/v2/cmd/golangci-lint': PrebuiltTool(
        repo='golangci/golangci-lint',
        binary_name='golangci-lint',
        asset_pattern='golangci-lint-{version}-{os}-{arch}.tar.gz',
        archive_binary_path='golangci-lint-{version}-{os}-{arch}/golangci-lint',
        go_mod_module='github.com/golangci/golangci-lint/v2',
    ),
    'gotest.tools/gotestsum': PrebuiltTool(
        repo='gotestyourself/gotestsum',
        binary_name='gotestsum',
        asset_pattern='gotestsum_{version}_{os}_{arch}.tar.gz',
        go_mod_module='gotest.tools/gotestsum',
    ),
    'github.com/vektra/mockery/v3': PrebuiltTool(
        repo='vektra/mockery',
        binary_name='mockery',
        # mockery uses capitalized OS name and x86_64 instead of amd64
        asset_pattern='mockery_{version}_{Os}_{arch}.tar.gz',
        go_mod_module='github.com/vektra/mockery/v3',
        arch_map={'amd64': 'x86_64'},
    ),
}

# Map of go.mod dir -> parsed module versions, lazily populated
_go_mod_versions_cache: dict[str, dict[str, str]] = {}


def _parse_go_mod_versions(go_mod_path: str) -> dict[str, str]:
    """Parse a go.mod file and return a dict of module -> version."""
    versions = {}
    with open(go_mod_path) as f:
        for line in f:
            m = re.match(r'\s+(\S+)\s+(v\S+)', line)
            if m:
                versions[m.group(1)] = m.group(2)
    return versions


def _get_tool_version(tool_import: str, go_mod_dir: str) -> str:
    """Get the pinned version of a tool from its go.mod."""
    if go_mod_dir not in _go_mod_versions_cache:
        _go_mod_versions_cache[go_mod_dir] = _parse_go_mod_versions(os.path.join(go_mod_dir, 'go.mod'))
    versions = _go_mod_versions_cache[go_mod_dir]
    prebuilt = PREBUILT_TOOLS[tool_import]
    mod = prebuilt.go_mod_module
    if mod in versions:
        return versions[mod]
    raise Exit(f"Could not find version for {mod} in {go_mod_dir}/go.mod", code=1)


def _format_asset_fields(pattern: str, version: str, os_name: str, arch: str) -> str:
    """Format an asset pattern with version/os/arch, supporting {Os} for capitalized OS."""
    return pattern.format(
        version=version,
        os=os_name,
        Os=os_name.capitalize(),
        arch=arch,
    )


def _install_prebuilt_tool(ctx: Context, tool_import: str, gobin: str, go_mod_dir: str):
    """Download a pre-built binary from GitHub Releases and place it in GOBIN."""
    prebuilt = PREBUILT_TOOLS[tool_import]
    version = _get_tool_version(tool_import, go_mod_dir)
    version_no_v = version.lstrip('v')
    dest = os.path.join(gobin, prebuilt.binary_name)

    os_name = 'darwin' if sys.platform.startswith('darwin') else 'linux'
    arch = 'arm64' if platform.machine() in ('arm64', 'aarch64') else 'amd64'
    if prebuilt.arch_map and arch in prebuilt.arch_map:
        arch = prebuilt.arch_map[arch]

    asset = _format_asset_fields(prebuilt.asset_pattern, version_no_v, os_name, arch)
    url = f"https://github.com/{prebuilt.repo}/releases/download/{version}/{asset}"

    print(f"Downloading {prebuilt.binary_name} {version} from {url}")

    if not prebuilt.is_archive:
        # Raw binary download
        ctx.run(f'curl -fsSL --retry 4 -o {dest} {url}')
        ctx.run(f'chmod +x {dest}')
    else:
        # Archive: extract the binary
        binary_in_archive = prebuilt.binary_name
        if prebuilt.archive_binary_path:
            binary_in_archive = _format_asset_fields(prebuilt.archive_binary_path, version_no_v, os_name, arch)

        with tempfile.TemporaryDirectory() as tmpdir:
            archive_path = os.path.join(tmpdir, 'archive.tar.gz')
            ctx.run(f'curl -fsSL --retry 4 -o {archive_path} {url}')
            ctx.run(f'tar xzf {archive_path} -C {tmpdir} {binary_in_archive}')
            extracted = os.path.join(tmpdir, binary_in_archive)
            ctx.run(f'mv {extracted} {dest}')
            ctx.run(f'chmod +x {dest}')

    print(f"Installed {prebuilt.binary_name} {version} to {dest}")


@task
def download_tools(ctx):
    """Download all Go tools for testing."""
    print(color_message("This command is deprecated, please use `install-tools` instead", Color.ORANGE))
    with environ({'GO111MODULE': 'on'}):
        download_go_dependencies(ctx, paths=list(TOOLS.keys()))


@task
def install_tools(ctx: Context, max_retry: int = 3, use_prebuilt: bool = True):
    """Install all Go tools for testing.

    Args:
        max_retry: Number of retries for go install commands.
        use_prebuilt: Download pre-built binaries for supported tools instead of
            compiling from source with ``go install``. Defaults to True.
    """
    with gitlab_section("Installing Go tools", collapsed=True):
        gobin = get_gobin(ctx)
        os.makedirs(gobin, exist_ok=True)

        env = {'GO111MODULE': 'on'}
        if os.getenv('DD_CC'):
            env['CC'] = os.getenv('DD_CC')
        if os.getenv('DD_CXX'):
            env['CXX'] = os.getenv('DD_CXX')
        with environ(env):
            for path, tools in TOOLS.items():
                with ctx.cd(path):
                    for tool in tools:
                        if use_prebuilt and tool in PREBUILT_TOOLS:
                            _install_prebuilt_tool(ctx, tool, gobin, path)
                        else:
                            run_command_with_retry(ctx, f"go install {tool}", max_retry=max_retry)
        for bazelisk in Path(gobin).glob('bazelisk*'):
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
