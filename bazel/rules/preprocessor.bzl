load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cpp_toolchain", "use_cc_toolchain")
load("@rules_cc//cc/common:cc_common.bzl", "cc_common")
load("@rules_cc//cc:action_names.bzl", "C_COMPILE_ACTION_NAME")

def _c_preprocessor_impl(ctx):
    out = ctx.outputs.output
    source = ctx.file.input
    cc_toolchain = find_cpp_toolchain(ctx)
    compilation_ctx = cc_common.create_compilation_context()

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
    )
    env = cc_common.get_environment_variables(
        feature_configuration = feature_configuration,
        action_name = C_COMPILE_ACTION_NAME,
        variables = c_compile_variables,
    )
    args = ctx.actions.args()
    args.add_all(["-E", "-P", source, "-o", out])
    ctx.actions.run(
        executable = compiler_path,
        arguments = [args],
        inputs = depset(
            [source],
            transitive = [cc_toolchain.all_files],
        ),
        outputs = [out],
    )
    return [
        DefaultInfo(files = depset([out])),
    ]


c_preprocessor = rule(
    implementation = _c_preprocessor_impl,
    attrs = {
        "input": attr.label(allow_single_file=True),
        "output": attr.output(mandatory=True),
    } | CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain(),
    fragments = ["cpp"],
)
