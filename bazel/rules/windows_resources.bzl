"""Rules for compiling Windows resource files (.mc -> .rc -> .syso).

windres internally calls gcc -E to preprocess .rc files via popen().
Under Bazel's --incompatible_strict_action_env the stripped environment
breaks popen (windres is a MinGW64/CRT binary whose popen needs cmd.exe,
which isn't in the MinGW-only PATH). We work around this with
--use-temp-file, which bypasses popen entirely.

We still resolve the CC toolchain to get the correct PATH for gcc itself.
"""

load("@bazel_lib//lib:copy_file.bzl", "COPY_FILE_TOOLCHAINS", "copy_file_action")
load("@rules_cc//cc:action_names.bzl", "C_COMPILE_ACTION_NAME")
load("@rules_cc//cc:defs.bzl", "cc_common")
load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cc_toolchain", "use_cc_toolchain")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("//bazel/rules:version_info.bzl", "agent_version_defines")

_WINDRES = "@winlibs_mingw64//:windres"
_WINDMC = "@winlibs_mingw64//:windmc"

def _cc_env(ctx):
    """Returns (env, cc_toolchain, feature_configuration) from the resolved CC toolchain."""
    cc_toolchain = find_cc_toolchain(ctx)
    feature_configuration = cc_common.configure_features(
        ctx = ctx,
        cc_toolchain = cc_toolchain,
        requested_features = ctx.features,
        unsupported_features = ctx.disabled_features,
    )
    env = cc_common.get_environment_variables(
        feature_configuration = feature_configuration,
        action_name = C_COMPILE_ACTION_NAME,
        variables = cc_common.empty_variables(),
    )
    return env, cc_toolchain, feature_configuration

def _syso_cc_info(ctx, syso_out, cc_toolchain, feature_configuration):
    """Wraps a .syso object file in a CcInfo so cc/rust link_deps can consume it.

    cc_common.create_library_to_link requires a prebuilt static/dynamic library
    artifact -- it can't wrap a bare object file on its own. Archiving the
    object via create_linking_context_from_compilation_outputs (the same path
    cc_library uses for precompiled .o srcs) sidesteps that.

    create_compilation_outputs only accepts objects named *.o/.obj/.opb/.bc, but
    the .syso extension is load-bearing for Go's build (it auto-links
    *_windows.syso files), so we copy it to a .o-suffixed alias for cc_common's sake
    rather than renaming the actual windres output.

    alwayslink is mandatory: the archived object defines no symbols, so a normal
    archive member is never pulled in by the linker and the resources silently
    vanish from the binary.
    """
    obj_alias = ctx.actions.declare_file(ctx.label.name + "_syso_alias.o")
    copy_file_action(ctx, syso_out, obj_alias)
    objects = depset([obj_alias])
    compilation_outputs = cc_common.create_compilation_outputs(objects = objects, pic_objects = objects)
    linking_context, _linking_outputs = cc_common.create_linking_context_from_compilation_outputs(
        actions = ctx.actions,
        name = ctx.label.name,
        feature_configuration = feature_configuration,
        cc_toolchain = cc_toolchain,
        compilation_outputs = compilation_outputs,
        disallow_dynamic_library = True,
        alwayslink = True,
    )
    return CcInfo(linking_context = linking_context)

def _windres(ctx, src, syso_out, inputs, args, progress_message):
    env, cc_toolchain, feature_configuration = _cc_env(ctx)

    windres_args = ctx.actions.args()
    windres_args.add("--use-temp-file")
    windres_args.add("--target", "pe-x86-64")
    windres_args.add_all(args)
    windres_args.add("-i", src)
    windres_args.add("-O", "coff")
    windres_args.add("-o", syso_out)

    ctx.actions.run(
        executable = ctx.executable._windres,
        arguments = [windres_args],
        env = env,
        inputs = depset([src] + inputs, transitive = [cc_toolchain.all_files]),
        outputs = [syso_out],
        mnemonic = "WindRes",
        progress_message = progress_message,
    )

    return _syso_cc_info(ctx, syso_out, cc_toolchain, feature_configuration)

# --- win_messagetable ---------------------------------------------------------

def _win_messagetable_impl(ctx):
    src = ctx.file.src

    # Pick output names based on the input file name rather than the
    # canonical ctx.label.name. WindMC always writes output that matches
    # the input file name. There is no option to set the output name.
    basename = src.basename.replace(".mc", "")
    rc_out = ctx.actions.declare_file(basename + ".rc")
    h_out = ctx.actions.declare_file(basename + ".h")
    bin_out = ctx.actions.declare_file("MSG00409.bin")

    windmc_args = ctx.actions.args()
    windmc_args.add("--target", "pe-x86-64")
    windmc_args.add("-r", rc_out.dirname)
    windmc_args.add("-h", rc_out.dirname)
    windmc_args.add(src)

    ctx.actions.run(
        executable = ctx.executable._windmc,
        arguments = [windmc_args],
        inputs = [src],
        outputs = [rc_out, h_out, bin_out],
        mnemonic = "WindMC",
        progress_message = "Compiling message table %s" % src.short_path,
    )

    syso_out = ctx.actions.declare_file(basename + ".syso")
    cc_info = _windres(ctx, rc_out, syso_out, [bin_out], [], "Linking message resource {}".format(rc_out.short_path))
    return [DefaultInfo(files = depset([syso_out, h_out])), cc_info]

_win_messagetable = rule(
    implementation = _win_messagetable_impl,
    doc = "Compiles a .mc message file into a .syso resource and .h header via windmc + windres.",
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".mc"]),
        "_windmc": attr.label(default = _WINDMC, executable = True, cfg = "exec", allow_single_file = True),
        "_windres": attr.label(default = _WINDRES, executable = True, cfg = "exec", allow_single_file = True),
    } | CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain() + COPY_FILE_TOOLCHAINS,
    fragments = ["cpp"],
)

def _win_messagetable_macro_impl(name, visibility, **kwargs):
    _win_messagetable(
        name = name,
        visibility = visibility,
        **kwargs
    )

win_messagetable = macro(
    doc = "Compiles a .mc message file into a .syso resource and .h header via windmc + windres.",
    inherit_attrs = _win_messagetable,
    attrs = {
        "target_compatible_with": attr.label_list(default = ["@platforms//os:windows"]),
    },
    implementation = _win_messagetable_macro_impl,
)

# --- win_resource -------------------------------------------------------------

def _win_resource_impl(ctx):
    src = ctx.file.src
    syso_out = ctx.actions.declare_file("{}.syso".format(ctx.label.name))

    args = []
    for k, v in ctx.attr.defines.items():
        args.extend(["--define", "{}={}".format(k, v)])

    include_dirs = {src.dirname: True}
    for dep in ctx.files.deps:
        include_dirs[dep.dirname] = True
    for d in include_dirs:
        args.extend(["-I", d])

    cc_info = _windres(ctx, src, syso_out, ctx.files.deps, args, "Linking resource {}".format(src.short_path))
    return [DefaultInfo(files = depset([syso_out])), cc_info]

_win_resource = rule(
    implementation = _win_resource_impl,
    doc = "Compiles a .rc resource file into a .syso object via windres.",
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".rc"]),
        "deps": attr.label_list(allow_files = True),
        "defines": attr.string_dict(),
        "_windres": attr.label(default = _WINDRES, executable = True, cfg = "exec", allow_single_file = True),
    } | CC_TOOLCHAIN_ATTRS,
    toolchains = use_cc_toolchain() + COPY_FILE_TOOLCHAINS,
    fragments = ["cpp"],
)

def _win_resource_macro_impl(name, visibility, **kwargs):
    _win_resource(
        name = name,
        visibility = visibility,
        **kwargs
    )

win_resource = macro(
    doc = "Compiles a .rc resource file into a .syso object via windres.",
    inherit_attrs = _win_resource,
    attrs = {
        "target_compatible_with": attr.label_list(default = ["@platforms//os:windows"]),
        "defines": attr.string_dict(default = agent_version_defines()),
    },
    implementation = _win_resource_macro_impl,
)
