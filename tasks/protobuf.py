import glob
import os
import re
from pathlib import Path

from invoke import Exit, UnexpectedExit, task

from tasks.install_tasks import TOOL_LIST_PROTO
from tasks.libs.common.check_tools_version import check_tools_installed
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_unstaged_files, get_untracked_files

PROTO_PKGS = {
    'model/v1': False,
    'remoteconfig': False,
    'api/v1': False,
    'trace': True,
    'process': False,
    'workloadmeta': False,
    'languagedetection': False,
    'privateactionrunner': False,
    'remoteagent': False,
    'autodiscovery': False,
    'trace/idx': False,
    'workloadfilter': False,
    'stateful': False,
}

CLI_EXTRAS = {
    'trace/idx': '--go_opt=module=github.com/DataDog/datadog-agent',
    'privateactionrunner': '--go_opt=module=github.com/DataDog/datadog-agent',
}

VTPROTO_PACKAGES = {'stateful'}

VTPROTO_CLI_EXTRAS = {
    # Limit vtproto to marshal/unmarshal/size and enable pooling.
    'stateful': '--go-vtproto_opt=features=marshal+unmarshal+size+pool',
}


def _go_bin_dir():
    """
    Resolve the Go bin directory used by install_tasks.go installs.
    Preference order: GOBIN, GOPATH/bin, then ~/go/bin.
    """
    gobin = os.getenv("GOBIN")
    if gobin:
        return gobin
    gopath = os.getenv("GOPATH")
    if gopath:
        return os.path.join(gopath, "bin")
    return os.path.join(Path.home(), "go", "bin")


def _plugin_path(env_var: str, default_name: str) -> str:
    """
    Allow an absolute override via env; otherwise use the default binary
    name resolved under the Go bin directory.
    """
    override = os.getenv(env_var)
    if override:
        return override
    candidate = os.path.join(_go_bin_dir(), default_name)
    if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
        return candidate
    # Fallback: let protoc resolve from PATH using the default_name.
    return default_name


def _vtprotobuf_include_paths() -> list[str]:
    """
    Return optional include paths for vtprotobuf's bundled protos.
    """
    include_paths: list[str] = []
    mod_cache = os.getenv("GOMODCACHE")
    if not mod_cache:
        gopath = os.getenv("GOPATH") or os.path.join(Path.home(), "go")
        mod_cache = os.path.join(gopath, "pkg", "mod")
    if os.path.isdir(mod_cache):
        candidates = glob.glob(os.path.join(mod_cache, "github.com", "planetscale", "vtprotobuf@*", "include"))
        if candidates:
            include_paths.append(sorted(candidates, reverse=True)[0])
    return include_paths


# Allow selecting specific protoc-gen-go binaries for legacy vs. new API.
# Default legacy plugin falls back to the standard name if the legacy-named
# binary is not present to avoid execution errors.
GO_PLUGIN_LEGACY = _plugin_path("PROTOC_GEN_GO_V1", "protoc-gen-go-legacy")
GO_PLUGIN_V2 = _plugin_path("PROTOC_GEN_GO_V2", "protoc-gen-go")
GO_PLUGIN_GRPC = _plugin_path("PROTOC_GEN_GO_GRPC", "protoc-gen-go-grpc")

GO_PLUGIN_OPTS = ""
GO_GRPC_PLUGIN_OPTS = ""
VTPROTO_PLUGIN_OPTS = ""

# protoc-go-inject-tag targets
INJECT_TAG_TARGETS = {
    'trace': ['span.pb.go', 'stats.pb.go', 'tracer_payload.pb.go', 'agent_payload.pb.go'],
}


@task
def generate(ctx, pre_commit=False):
    """
    Generates protobuf definitions in pkg/proto

    We must build the packages one at a time due to protoc-gen-go limitations
    """
    proto_file = re.compile(r"pkg/proto/pbgo/.*\.pb\.go$")
    old_unstaged_proto_files = set(get_unstaged_files(ctx, re_filter=proto_file, include_deleted_files=True))
    old_untracked_proto_files = set(get_untracked_files(ctx, re_filter=proto_file))
    # Key: path, Value: inject_tags
    check_tools(ctx)
    base = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(base, ".."))
    proto_root = os.path.join(repo_root, "pkg", "proto")
    protodep_root = os.path.join(proto_root, "protodep")
    pbgo_dir = os.path.join(proto_root, "pbgo")
    print(f"nuking old definitions at: {proto_root}")
    file_list = glob.glob(os.path.join(proto_root, "pbgo", "*.pb.go"))
    for file_path in file_list:
        try:
            os.remove(file_path)
        except OSError:
            print("Error while deleting file : ", file_path)

    with ctx.cd(repo_root):
        # protobuf defs
        print(f"generating protobuf code from: {proto_root}")

        for pkg, inject_tags in PROTO_PKGS.items():
            files = []
            pkg_root = os.path.join(proto_root, "datadog", pkg).rstrip(os.sep)
            pkg_root_level = pkg_root.count(os.sep)
            for path in Path(pkg_root).rglob('*.proto'):
                if path.as_posix().count(os.sep) == pkg_root_level + 1:
                    files.append(path.as_posix())

            targets = ' '.join(files)

            # Generate Go/grpc stubs (protobuf API v2) and optional vtproto helpers.
            go_cli_extras = CLI_EXTRAS.get(pkg, '')
            include_args = [f"-I{proto_root}", f"-I{protodep_root}"]
            include_args += [f"-I{path}" for path in _vtprotobuf_include_paths()]
            go_opt_args = []
            if "module=" not in go_cli_extras and GO_PLUGIN_OPTS:
                go_opt_args.append(GO_PLUGIN_OPTS)
            if go_cli_extras:
                go_opt_args.append(go_cli_extras)

            if pkg in VTPROTO_PACKAGES:
                # New API v2 flow: separate go and go-grpc outputs.
                go_arg_list = [
                    "protoc",
                    *include_args,
                    f"--plugin=protoc-gen-go={GO_PLUGIN_V2}",
                    f"--plugin=protoc-gen-go-grpc={GO_PLUGIN_GRPC}",
                    f"--go_out={repo_root}",
                    *go_opt_args,
                    f"--go-grpc_out={repo_root}",
                ]
                if GO_GRPC_PLUGIN_OPTS:
                    go_arg_list.append(GO_GRPC_PLUGIN_OPTS)
            else:
                # Legacy grpc plugin for non-vtproto packages.
                go_arg_list = [
                    "protoc",
                    *include_args,
                    f"--plugin=protoc-gen-go={GO_PLUGIN_LEGACY}",
                    f"--go_out=plugins=grpc:{repo_root}",
                    *go_opt_args,
                ]

            go_arg_list.append(targets)
            ctx.run(" ".join(go_arg_list))

            if pkg in VTPROTO_PACKAGES:
                vt_cli_extras = VTPROTO_CLI_EXTRAS.get(pkg, '')
                vt_arg_list = [
                    "protoc",
                    *include_args,
                    f"--go-vtproto_out={repo_root}",
                ]
                if VTPROTO_PLUGIN_OPTS:
                    vt_arg_list.append(VTPROTO_PLUGIN_OPTS)
                if vt_cli_extras:
                    vt_arg_list.append(vt_cli_extras)
                vt_arg_list.append(targets)
                ctx.run(" ".join(vt_arg_list))

            if inject_tags:
                inject_path = os.path.join(proto_root, "pbgo", pkg)
                # inject_tags logic
                for target in INJECT_TAG_TARGETS[pkg]:
                    ctx.run(f"protoc-go-inject-tag -input={os.path.join(inject_path, target)}")

        # Mockgen (not done in pre-commit as it is slow)
        if not pre_commit:
            mockgen_out = os.path.join(proto_root, "pbgo", "mocks")
            pbgo_rel = os.path.relpath(pbgo_dir, repo_root)
            try:
                os.mkdir(mockgen_out)
            except FileExistsError:
                print(f"{mockgen_out} folder already exists")

            ctx.run(f"mockgen -source={pbgo_rel}/core/api.pb.go -destination={mockgen_out}/core/api_mockgen.pb.go")

    # Generate messagepack marshallers
    # msgp targets (file, io)
    msgp_targets = {
        'trace': [
            ('trace.go', False),
            ('span.pb.go', False),
            ('stats.pb.go', True),
            ('tracer_payload.pb.go', False),
            ('agent_payload.pb.go', False),
        ],
        'core': [('remoteconfig.pb.go', False)],
    }
    for pkg, files in msgp_targets.items():
        for src, io_gen in files:
            dst = os.path.splitext(os.path.basename(src))[0]  # .go
            dst = os.path.splitext(dst)[0]  # .pb
            ctx.run(f"msgp -file {pbgo_dir}/{pkg}/{src} -o={pbgo_dir}/{pkg}/{dst}_gen.go -io={io_gen}")

    # Apply msgp patches
    # msgp patches key is `pkg` : (patch, destination)
    #     if `destination` is `None` diff will target inherent patch files
    msgp_patches = {
        'trace': [
            ('0001-Customize-msgpack-parsing.patch', '-p4'),
            ('0002-Make-nil-map-deserialization-retrocompatible.patch', '-p4'),
        ],
    }
    for pkg, patches in msgp_patches.items():
        for patch in patches:
            patch_file = os.path.join(proto_root, "patches", patch[0])
            switches = patch[1] if patch[1] else ''
            ctx.run(f"git apply {switches} --unsafe-paths --directory='{pbgo_dir}/{pkg}' {patch_file}")

    # Check the generated files were properly committed
    current_unstaged_proto_files = set(get_unstaged_files(ctx, re_filter=proto_file, include_deleted_files=True))
    current_untracked_proto_files = set(get_untracked_files(ctx, re_filter=proto_file))
    if (
        old_unstaged_proto_files != current_unstaged_proto_files
        or old_untracked_proto_files != current_untracked_proto_files
    ):
        if pre_commit:
            updated_files = [f"- {file}\n" for file in current_unstaged_proto_files - old_unstaged_proto_files]
            updated_files += [f"- {file}\n" for file in current_untracked_proto_files - old_untracked_proto_files]
            raise Exit(f"Files modified\n{''.join(updated_files)}", code=1)
        else:
            print("Generation complete and new files were updated, don't forget to commit and push")
    else:
        print(f"[{color_message('WARN', Color.ORANGE)}] Generation complete and no new files were updated")


def check_tools(ctx):
    """
    Check if all the required dependencies are installed
    """
    tools = [tool.split("/")[-1] for tool in TOOL_LIST_PROTO]
    if not check_tools_installed(tools):
        raise Exit("Please install the required tools with `dda inv install-tools` before running this task.", code=1)
    try:
        current_version = ctx.run("protoc --version", hide=True).stdout.strip().removeprefix("libprotoc ")
        with open(".protoc-version") as f:
            expected_version = f.read().strip()
        if current_version != expected_version:
            raise Exit(
                f"Expected protoc version {expected_version}, found {current_version}. Please run `dda inv install-protoc` before running this task.",
                code=1,
            )
    except UnexpectedExit as e:
        raise Exit("protoc is not installed. Please install it before running this task.", code=1) from e
