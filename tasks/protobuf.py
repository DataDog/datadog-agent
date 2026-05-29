import os
import re

from invoke import Exit, task

from tasks.libs.build.bazel import BazelTools, run_bazel
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_unstaged_files, get_untracked_files


@task
def generate(ctx, pre_commit=False):
    """
    Generates protobuf definitions in pkg/proto

    We must build the packages one at a time due to protoc-gen-go limitations
    """
    proto_file = re.compile(r"pkg/proto/pbgo/.*\.pb\.go$")
    old_unstaged_proto_files = set(get_unstaged_files(ctx, re_filter=proto_file, include_deleted_files=True))
    old_untracked_proto_files = set(get_untracked_files(ctx, re_filter=proto_file))
    bt = BazelTools(ctx)
    base = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(base, ".."))
    proto_root = os.path.join(repo_root, "pkg", "proto")
    pbgo_dir = os.path.join(proto_root, "pbgo")

<<<<<<< HEAD
    # protobuf defs
    print(f"generating protobuf code from: {proto_root}")
    # Find all the instances created with bazel/rules/write_pb_go/defs.bzl
    # This query captures too many things: the primary target + one for each .proto file.
    # but write_source_files puts kwargs.tag in sub rules rather than just the top, so
    # we need to filter out the extras.
    result = run_bazel(
        ctx,
        "query",
        """attr(tags, "write_pb_go", kind("_write_source_file", //pkg/proto/...))""",
    )
    for target in result.stdout.split("\n"):
        # We could look for the regex '.*_[0-9]+' and filter that, but this is good enough.
        if target.endswith("_pb_go"):
            result = run_bazel(ctx, "run", target, verbose=True)
            if result.return_code != 0:
                print(result.stderr)

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
    # Per-file extra directives, keyed by (pkg, src).
    # stats.pb.go is protoc-generated so the limit directive cannot live in the
    # file itself; pass it on the command line instead.
    msgp_file_directives = {
        ('trace', 'stats.pb.go'): '-d "limit arrays:500000 maps:500000"',
        ('trace', 'span.pb.go'): '-d "limit arrays:500000 maps:500000"',
        ('trace', 'tracer_payload.pb.go'): '-d "limit arrays:500000 maps:500000"',
        ('trace', 'agent_payload.pb.go'): '-d "limit arrays:500000 maps:500000"',
    }
    for pkg, files in msgp_targets.items():
        for src, io_gen in files:
            dst = os.path.splitext(os.path.basename(src))[0]  # .go
            dst = os.path.splitext(dst)[0]  # .pb
            extra_flags = msgp_file_directives.get((pkg, src), '')
            ctx.run(
                f"{bt.msgp} -file {pbgo_dir}/{pkg}/{src} -o={pbgo_dir}/{pkg}/{dst}_gen.go -io={io_gen} {extra_flags}",
                env=bt.go_env,
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
