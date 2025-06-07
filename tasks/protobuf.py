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
    'remoteagent': False,
    'autodiscovery': False,
}

# maybe put this in a separate function
PKG_PLUGINS = {
    'trace': '--go-vtproto_out=',
}

PKG_CLI_EXTRAS = {
    'trace': '--go-vtproto_opt=features=marshal+unmarshal+size',
}

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

            # output_generator could potentially change for some packages
            # so keep it in a variable for sanity.
            output_generator = "--go_out=plugins=grpc:"
            cli_extras = ''
            ctx.run(f"protoc -I{proto_root} -I{protodep_root} {output_generator}{repo_root} {cli_extras} {targets}")

            if pkg in PKG_PLUGINS:
                output_generator = PKG_PLUGINS[pkg]

                if pkg in PKG_CLI_EXTRAS:
                    cli_extras = PKG_CLI_EXTRAS[pkg]

                ctx.run(f"protoc -I{proto_root} -I{protodep_root} {output_generator}{repo_root} {cli_extras} {targets}")

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
