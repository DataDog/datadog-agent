load("@bazel_skylib//lib:paths.bzl", "paths")
load("@rules_cc//cc:action_names.bzl", "C_COMPILE_ACTION_NAME")
load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cpp_toolchain", "use_cc_toolchain")
load("@rules_cc//cc/common:cc_common.bzl", "cc_common")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")

def _c_preprocessor_impl(ctx):
    out = ctx.outputs.output
    source = ctx.file.input
    include_dirs = [paths.dirname(f.path) for f in ctx.files.deps]
    cc_toolchain = find_cpp_toolchain(ctx)
    compilation_ctx = cc_common.create_compilation_context(
        headers = depset(ctx.files.deps),
    )

    feature_configuration = cc_common.configure_features(
        ctx = ctx,
        cc_toolchain = cc_toolchain,
        requested_features = ctx.features,
        unsupported_features = ctx.disabled_features,
    )
    compiler_path = cc_common.get_tool_for_action(
        feature_configuration = feature_configuration,
        action_name = C_COMPILE_ACTION_NAME,
    )
    args = ctx.actions.args()
    args.add_all(["-E", "-P", source, "-o", out])
    args.add_all(include_dirs, before_each = "-I")
    ctx.actions.run(
        executable = compiler_path,
        arguments = [args],
        inputs = depset(
            [source] + ctx.files.deps,
            transitive = [cc_toolchain.all_files],
        ),
        outputs = [out],
    )
    cc_info = cc_common.merge_cc_infos(cc_infos = [
        CcInfo(compilation_context = compilation_ctx),
    ])
    return [cc_info]

c_preprocessor = rule(
    implementation = _c_preprocessor_impl,
    attrs = {
        "input": attr.label(allow_single_file = True),
        "deps": attr.label_list(allow_files = [".h"]),
        "output": attr.output(mandatory = True),
        "include_directories": attr.string_list(),
    } | CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain(),
    fragments = ["cpp"],
)
