load("@rules_cc//cc:action_names.bzl", "OBJ_COPY_ACTION_NAME")
load("@rules_cc//cc:defs.bzl", "cc_common")
load("@rules_cc//cc:find_cc_toolchain.bzl", "find_cc_toolchain", "use_cc_toolchain")

# Bazel's default --strip=sometimes strips rules_go binaries in fastbuild mode.
# These fixture binaries deliberately exercise symbol/debug-section handling, so
# build them with --strip=never and let the test-specific objcopy action below
# apply the exact transformations the test needs.
def _strip_never_transition_impl(_settings, _attr):
    return {"//command_line_option:strip": "never"}

_strip_never_transition = transition(
    implementation = _strip_never_transition_impl,
    inputs = [],
    outputs = ["//command_line_option:strip"],
)

def _objcopy_go_fixture_impl(ctx):
    cc_toolchain = find_cc_toolchain(ctx)
    feature_configuration = cc_common.configure_features(
        ctx = ctx,
        cc_toolchain = cc_toolchain,
        requested_features = [],
        unsupported_features = [],
    )
    objcopy = cc_common.get_tool_for_action(
        feature_configuration = feature_configuration,
        action_name = OBJ_COPY_ACTION_NAME,
    )

    args = ctx.actions.args()
    args.add_all(ctx.attr.objcopy_args)
    args.add(ctx.file.src)
    args.add(ctx.outputs.out)

    ctx.actions.run(
        executable = objcopy,
        inputs = depset(
            direct = [ctx.file.src],
            transitive = [cc_toolchain.all_files],
        ),
        outputs = [ctx.outputs.out],
        arguments = [args],
        mnemonic = "ObjcopyGoFixture",
        progress_message = "Post-processing Go fixture {}".format(ctx.label),
    )

    return [DefaultInfo(files = depset([ctx.outputs.out]))]

def _cc_toolchain_objcopy_impl(ctx):
    cc_toolchain = find_cc_toolchain(ctx)
    feature_configuration = cc_common.configure_features(
        ctx = ctx,
        cc_toolchain = cc_toolchain,
        requested_features = [],
        unsupported_features = [],
    )
    objcopy_path = cc_common.get_tool_for_action(
        feature_configuration = feature_configuration,
        action_name = OBJ_COPY_ACTION_NAME,
    )

    objcopy = None
    for f in cc_toolchain.all_files.to_list():
        if f.path == objcopy_path:
            objcopy = f
            break
    if objcopy == None:
        fail("objcopy ({}) is not a file provided by the cc toolchain, it cannot be exposed to a test as a runfile".format(objcopy_path))

    # The whole toolchain is carried in the runfiles: objcopy may be a wrapper
    # script, or link against libraries shipped by the toolchain.
    return [DefaultInfo(
        files = depset([objcopy]),
        runfiles = ctx.runfiles(transitive_files = cc_toolchain.all_files),
    )]

cc_toolchain_objcopy = rule(
    implementation = _cc_toolchain_objcopy_impl,
    doc = """Exposes the cc toolchain's objcopy to a test as a runfile.

Depend on this target from a test's `data` and pass its location through `env`
with `$(rlocationpath ...)`, so the test runs the same objcopy as the build
instead of whichever one happens to be on PATH.""",
    fragments = ["cpp"],
    toolchains = use_cc_toolchain(),
)

objcopy_go_fixture = rule(
    implementation = _objcopy_go_fixture_impl,
    attrs = {
        "src": attr.label(
            allow_single_file = True,
            cfg = _strip_never_transition,
            mandatory = True,
        ),
        "out": attr.output(mandatory = True),
        "objcopy_args": attr.string_list(),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
    },
    fragments = ["cpp"],
    toolchains = use_cc_toolchain(),
)
