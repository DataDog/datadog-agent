"""Make a rules_foreign_cc tree runnable from Bazel actions.

`configure_make` outputs don't come out with rpaths pointing at where their dependencies
would be found under Bazel's output tree.
This rule produces a copy of the tree with rpaths rewritten so that:

  - Intra-tree shared libs (e.g. libpython in the tree's own `lib/`) are found
    via `$ORIGIN`-relative paths from each binary/shared lib.
  - Cross-tree shared libs (from the input's dynamic deps) are found at their
    canonical bazel-out paths, which Bazel stages in any consumer action that
    receives this rule's output.

Most of the necessary information to decide which files to patch and what rpath entries
to add are based on CcInfo and DefaultInfo entries produced by `rules_foreign_cc` rules.
"""

load("@bazel_lib//lib:paths.bzl", "relative_file")
load("@bazel_skylib//lib:paths.bzl", "paths")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")

def _relative_folder(to_dir, from_path):
    """Relative path between folders."""

    # bazel_lib's relative_file assumes files, to make it work for folders
    # we need to go up one more level.
    return paths.normalize("../" + relative_file(to_dir, from_path))

def _is_os(ctx, constraint):
    return ctx.target_platform_has_constraint(constraint[platform_common.ConstraintValueInfo])

def _collect_cc_libs(cc_info, input_label):
    """Collect libs from CcInfo's linker inputs."""
    own_libs = []
    dep_libs = []
    for linker_input in cc_info.linking_context.linker_inputs.to_list():
        for lib in linker_input.libraries:
            f = lib.resolved_symlink_dynamic_library or lib.dynamic_library
            if f == None:
                continue
            if linker_input.owner == input_label:
                own_libs.append(f)
            else:
                dep_libs.append(f)
    return own_libs, dep_libs

def _foreign_cc_runnable_impl(ctx):
    # rules_foreign_cc exposes the full install root via the `gen_dir` output
    # group; the default files list is a heterogeneous mix of binaries,
    # shared libs and data dirs.
    gen_dir_files = ctx.attr.input[OutputGroupInfo].gen_dir.to_list()
    if len(gen_dir_files) != 1:
        fail("Expected a single `gen_dir` tree artifact on {}, got {}".format(
            ctx.attr.input.label,
            len(gen_dir_files),
        ))
    input_tree = gen_dir_files[0]
    output_tree = ctx.actions.declare_directory(ctx.label.name)

    # Canonical path to the root of the install tree for individual file outputs.
    install_root = paths.join(
        input_tree.root.path,
        ctx.attr.input.label.workspace_root,
        ctx.attr.input.label.package,
        ctx.attr.input.label.name,
    )

    # Collect own and dependency libraries
    own_libs, dep_libs = _collect_cc_libs(ctx.attr.input[CcInfo], ctx.attr.input.label)

    # Tree-relative directories that should be in every patched file's rpath,
    # based on the libraries coming from the input itself as well as those in dependencies.
    own_dirs = set([paths.dirname(paths.relativize(f.path, install_root)) for f in own_libs])
    dep_dirs = set([f.dirname for f in dep_libs])
    rpath_dirs = sorted(own_dirs) + sorted([_relative_folder(d, output_tree.path) for d in dep_dirs])

    # Build manifest instructions. Each line:
    #   FILE<TAB>tree-relative-path
    #   GLOB<TAB>tree-relative-pattern
    instructions = []

    # Always patch the rule's executable binary.
    instructions.append("FILE\t" + ctx.attr.binary_path)

    # Auto-discovered own shared libs.
    for lib in own_libs:
        instructions.append("FILE\t" + paths.relativize(lib.path, install_root))

    # Globs for contents of opaque TreeArtifacts that need to be patched.
    for glob in ctx.attr.patch_globs:
        instructions.append("GLOB\t" + glob)

    manifest = ctx.actions.declare_file(ctx.label.name + "_patch_manifest.txt")
    ctx.actions.write(manifest, "\n".join(instructions) + "\n")

    is_linux = _is_os(ctx, ctx.attr._linux_constraint)
    is_macos = _is_os(ctx, ctx.attr._macos_constraint)
    if not is_linux and not is_macos:
        fail("{}: unsupported platform (Linux and macOS only)".format(ctx.label))

    args = ctx.actions.args()

    tools = []
    if is_linux:
        patchelf_toolchain = ctx.toolchains["@@//bazel/toolchains/patchelf:patchelf_toolchain_type"].patchelf
        patchelf = patchelf_toolchain.label[DefaultInfo].files_to_run
        args.add("--patchelf", patchelf.executable.path)
        tools.append(patchelf)
    else:
        install_name_tool = ctx.executable._install_name_tool
        args.add("--install-name-tool", install_name_tool.path)
        tools.append(install_name_tool)

    args.add("linux" if is_linux else "darwin")
    args.add(input_tree.path)
    args.add(output_tree.path)
    args.add(manifest.path)
    args.add_all(rpath_dirs)

    ctx.actions.run(
        executable = ctx.file._script,
        arguments = [args],
        inputs = [input_tree, manifest],
        tools = tools,
        outputs = [output_tree],
        mnemonic = "ForeignCcRunnable",
        progress_message = "Rewriting rpaths for %{label}",
    )

    # Expose the chosen binary inside the tree as a separately-declared executable output,
    # so callers can use it as an `executable` without digging into the TreeArtifact.
    executable = ctx.actions.declare_symlink(ctx.label.name + "_bin")
    ctx.actions.symlink(
        output = executable,
        target_path = "{tree}/{path}".format(
            tree = ctx.label.name,
            path = ctx.attr.binary_path,
        ),
    )

    return DefaultInfo(
        files = depset([output_tree, executable] + dep_libs),
        executable = executable,
    )

foreign_cc_runnable = rule(
    implementation = _foreign_cc_runnable_impl,
    executable = True,
    doc = """Rewrite rpaths in a rules_foreign_cc tree so it can be used as a
    tool in Bazel actions. Cross-tree dependencies are derived from the
    input's CcInfo and bundled via DefaultInfo so consumers automatically
    stage them.""",
    attrs = {
        "input": attr.label(
            mandatory = True,
            providers = [OutputGroupInfo, CcInfo],
            doc = "A rules_foreign_cc target (configure_make/cmake) whose output tree should be made runnable.",
        ),
        "binary_path": attr.string(
            mandatory = True,
            doc = """Tree-relative path of the binary to expose as the rule's executable.
            A symlink at the rule's output level points into the patched tree at this path;
            the file itself is always patched.""",
        ),
        "patch_globs": attr.string_list(
            doc = """Tree-relative glob patterns matching additional files to patch.
            Use for files produced by the build which are not declared as libraries.""",
        ),
        "_script": attr.label(
            default = "//bazel/rules/foreign_cc_runnable:patch_rpaths.sh",
            allow_single_file = True,
            cfg = "exec",
            executable = True,
        ),
        "_install_name_tool": attr.label(
            default = "@@//bazel/tools:install_name_tool",
            executable = True,
            cfg = "exec",
        ),
        "_linux_constraint": attr.label(
            default = "@platforms//os:linux",
        ),
        "_macos_constraint": attr.label(
            default = "@platforms//os:macos",
        ),
    },
    toolchains = [
        "@@//bazel/toolchains/patchelf:patchelf_toolchain_type",
    ],
)
