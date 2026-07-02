import re

from invoke import Exit, task

from tasks.libs.build.bazel import run_bazel
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_unstaged_files, get_untracked_files


@task
def generate(ctx, pre_commit=False):
    """
    Generates protobuf definitions in pkg/proto
    """
    proto_file = re.compile(r".*(\.pb|_gen(_test)?)\.go$")
    old_unstaged_proto_files = set(get_unstaged_files(ctx, re_filter=proto_file, include_deleted_files=True))
    old_untracked_proto_files = set(get_untracked_files(ctx, re_filter=proto_file))

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
