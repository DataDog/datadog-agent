load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cpp_toolchain", "use_cc_toolchain")
load("@rules_cc//cc/common:cc_common.bzl", "cc_common")
load("@rules_cc//cc:action_names.bzl", "C_COMPILE_ACTION_NAME")

def _to_include_dir(ctx, path):
    workspace = ctx.label.workspace_root
    print(workspace)
    return path

def _c_preprocessor_impl(ctx):
    out = ctx.outputs.output
    source = ctx.file.input
    include_dirs = [_to_include_dir(ctx, f) for f in ctx.attr.include_directories]
    cc_toolchain = find_cpp_toolchain(ctx)
    compilation_ctx = cc_common.create_compilation_context(
        headers=depset(ctx.files.deps),
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
    c_compile_variables = cc_common.create_compile_variables(
        feature_configuration = feature_configuration,
        cc_toolchain = cc_toolchain,
        source_file = source.path,
        output_file = out.path,
        include_directories = depset(include_dirs),
    )
    env = cc_common.get_environment_variables(
        feature_configuration = feature_configuration,
        action_name = C_COMPILE_ACTION_NAME,
        variables = c_compile_variables,
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
        "input": attr.label(allow_single_file=True),
        "deps": attr.label_list(allow_files = [".h"]),
        "output": attr.output(mandatory=True),
        "include_directories": attr.string_list(),
    } | CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain(),
    fragments = ["cpp"],
)
