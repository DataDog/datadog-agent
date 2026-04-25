import glob
import os
import re
from pathlib import Path

from invoke import Exit, task

from tasks.libs.build.bazel import BazelTools
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_unstaged_files, get_untracked_files
from tasks.libs.common.utils import debug_go_proxy_env

PROTO_PKGS = {
    'model/v1': False,
    'remoteconfig': False,
    'api/v1': False,
    'trace': True,
    'process': False,
    'workloadmeta': False,
    'kubemetadata': False,
    'languagedetection': False,
    'privateactionrunner': False,
    'remoteagent': False,
    'autodiscovery': False,
    'trace/idx': False,
    'workloadfilter': False,
    'dogstatsdhttp': False,
    'sbom': False,
}

CLI_EXTRAS = {
    'trace/idx': '--go_opt=module=github.com/DataDog/datadog-agent',
    'privateactionrunner': '--go_opt=module=github.com/DataDog/datadog-agent',
}

CLI_EXTRAS_GRPC = {
    'trace/idx': '--go-grpc_opt=module=github.com/DataDog/datadog-agent',
    'privateactionrunner': '--go-grpc_opt=module=github.com/DataDog/datadog-agent',
}

# maybe put this in a separate function
PKG_PLUGINS = {
    'trace': '--go-vtproto_out=',
    'dogstatsdhttp': '--go-vtproto_out=',
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
    debug_go_proxy_env(ctx, "protobuf.generate")
    proto_file = re.compile(r"pkg/proto/pbgo/.*\.pb\.go$")
    old_unstaged_proto_files = set(get_unstaged_files(ctx, re_filter=proto_file, include_deleted_files=True))
    old_untracked_proto_files = set(get_untracked_files(ctx, re_filter=proto_file))
    bt = BazelTools(ctx)
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
            pkg_root = Path(proto_root, "datadog", pkg)
            for path in pkg_root.rglob('*.proto'):
                if len(path.parts) == len(pkg_root.parts) + 1:
                    files.append(path.as_posix())

            targets = ' '.join(files)

            # Generate Go code with protoc-gen-go and protoc-gen-go-grpc
            # Note: The new protoc-gen-go doesn't support plugins=grpc, so we use separate outputs
            # This generates *.pb.go (messages) and *_grpc.pb.go (gRPC stubs) in a single protoc call
            cli_extras = ''
            cli_extras_grpc = ''
            if pkg in CLI_EXTRAS:
                cli_extras = CLI_EXTRAS[pkg]
            if pkg in CLI_EXTRAS_GRPC:
                cli_extras_grpc = CLI_EXTRAS_GRPC[pkg]
            ctx.run(
                f"{bt.protoc} {bt.protoc_plugin("protoc-gen-go")} {bt.protoc_plugin("protoc-gen-go-grpc")} -I{proto_root} -I{protodep_root} --go_out={repo_root} {cli_extras} --go-grpc_out={repo_root} {cli_extras_grpc} {targets}"
            )

            if pkg in PKG_PLUGINS:
                output_generator = PKG_PLUGINS[pkg]

                if pkg in PKG_CLI_EXTRAS:
                    cli_extras = PKG_CLI_EXTRAS[pkg]

                ctx.run(
                    f"{bt.protoc} {bt.protoc_plugin("protoc-gen-go-vtproto")} -I{proto_root} -I{protodep_root} {output_generator}{repo_root} {cli_extras} {targets}"
                )

            if inject_tags:
                inject_path = os.path.join(proto_root, "pbgo", pkg)
                # inject_tags logic
                for target in INJECT_TAG_TARGETS[pkg]:
                    ctx.run(f"{bt.protoc_go_inject_tag} -input={os.path.join(inject_path, target)}")

        # Mockgen (not done in pre-commit as it is slow)
        if not pre_commit:
            mockgen_out = os.path.join(proto_root, "pbgo", "mocks")
            pbgo_rel = Path(pbgo_dir).relative_to(repo_root).as_posix()
            try:
                os.mkdir(mockgen_out)
            except FileExistsError:
                print(f"{mockgen_out} folder already exists")

            # Generate mocks from the gRPC file (api_grpc.pb.go) which contains the client/server interfaces
            ctx.run(
                f"{bt.mockgen} -source={pbgo_rel}/core/api_grpc.pb.go -destination={mockgen_out}/core/api_mockgen.pb.go",
                env=bt.go_env,
            )

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
            ctx.run(
                f"{bt.msgp} -file {pbgo_dir}/{pkg}/{src} -o={pbgo_dir}/{pkg}/{dst}_gen.go -io={io_gen}", env=bt.go_env
            )

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
            ctx.run(f'git apply {switches} --unsafe-paths --directory="{pbgo_dir}/{pkg}" {patch_file}')

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
