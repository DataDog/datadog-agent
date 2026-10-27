"""Rules for compiling Windows resource files (.mc -> .rc/.res -> .syso).

The message compiler depends on the exec platform: WinLibs' windmc on Windows, Wine's wmc on Linux.

LLVM's windres and its clang preprocessor run as `llvm windres` and `llvm clang`, since Windows containers cannot
start them through their symlinks on bind mounts (microsoft/Windows-Containers#634).
"""

load("@bazel_lib//lib:copy_file.bzl", "COPY_FILE_TOOLCHAINS", "copy_file_action")
load("@rules_cc//cc:defs.bzl", "CcInfo", "cc_common")
load("@rules_cc//cc:find_cc_toolchain.bzl", "CC_TOOLCHAIN_ATTRS", "find_cc_toolchain", "use_cc_toolchain")
load("//bazel/rules:version_info.bzl", "agent_version_defines")

def _syso_cc_info(ctx, syso_out):
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
    cc_toolchain = find_cc_toolchain(ctx)
    feature_configuration = cc_common.configure_features(
        ctx = ctx,
        cc_toolchain = cc_toolchain,
        requested_features = ctx.features,
        unsupported_features = ctx.disabled_features,
    )
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

def _windres(ctx, src, syso_out, inputs, args):
    windres_args = ctx.actions.args()
    windres_args.add("windres")
    windres_args.add("--target", "pe-x86-64")
    windres_args.add("--preprocessor", ctx.executable._llvm)
    windres_args.add_all(
        ["clang", "-target", "x86_64-w64-mingw32", "-E", "-xc", "-DRC_INVOKED"],
        format_each = "--preprocessor-arg=%s",
    )
    windres_args.add_all(args)
    windres_args.add("-i", src)
    windres_args.add("-O", "coff")
    windres_args.add("-o", syso_out)

    ctx.actions.run(
        executable = ctx.executable._llvm,
        arguments = [windres_args],
        inputs = [src] + inputs,
        outputs = [syso_out],
        mnemonic = "WindRes",
        progress_message = "Linking resource %{input}",
    )

    return _syso_cc_info(ctx, syso_out)

# --- win_messagetable ---------------------------------------------------------

def _win_messagetable_impl(ctx):
    src = ctx.file.src

    # Pick output names based on the input file name rather than the
    # canonical ctx.label.name. WindMC always writes output that matches
    # the input file name. There is no option to set the output name.
    basename = src.basename.replace(".mc", "")
    h_out = ctx.actions.declare_file(basename + ".h")

    if ctx.files._nls:
        res_out = ctx.actions.declare_file(basename + ".res")
        nls_dir = "--nls-dir=" + ctx.files._nls[0].dirname
        ctx.actions.run(
            executable = ctx.executable._mc,
            arguments = [nls_dir, "-O", "res", "-o", res_out.path, src.path],
            inputs = [src] + ctx.files._nls,
            outputs = [res_out],
            mnemonic = "Wmc",
            progress_message = "Compiling message table %{input} into %{output}",
        )
        ctx.actions.run(
            executable = ctx.executable._mc,
            arguments = [nls_dir, "-i", "-H", h_out.path, "-o", "/dev/null", src.path],  # discard .rc file
            inputs = [src] + ctx.files._nls,
            outputs = [h_out],
            mnemonic = "Wmc",
            progress_message = "Compiling message table %{input} into %{output}",
        )
        windres_src, windres_inputs = res_out, []
    else:
        rc_out = ctx.actions.declare_file(basename + ".rc")
        bin_out = ctx.actions.declare_file("MSG00409.bin")
        windmc_args = ctx.actions.args()
        windmc_args.add("--target", "pe-x86-64")
        windmc_args.add("-r", rc_out.dirname)
        windmc_args.add("-h", rc_out.dirname)
        windmc_args.add(src)

        ctx.actions.run(
            executable = ctx.executable._mc,
            arguments = [windmc_args],
            inputs = [src],
            outputs = [rc_out, h_out, bin_out],
            mnemonic = "WindMC",
            progress_message = "Compiling message table %{input}",
        )

        #TODO(agent-build): drop expand_template step once `llvm` ships https://github.com/llvm/llvm-project/pull/234376
        windres_src = ctx.actions.declare_file(basename + ".llvm.rc")
        ctx.actions.expand_template(output = windres_src, substitutions = {" MESSAGETABLE ": " 11 "}, template = rc_out)
        windres_inputs = [bin_out]

    syso_out = ctx.actions.declare_file(basename + ".syso")
    cc_info = _windres(ctx, windres_src, syso_out, windres_inputs, [])
    return [DefaultInfo(files = depset([syso_out, h_out])), cc_info]

_win_messagetable = rule(
    implementation = _win_messagetable_impl,
    doc = "Compiles a .mc message file into a .syso resource and .h header via windmc + windres.",
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".mc"]),
        "_llvm": attr.label(default = "//:llvm", executable = True, cfg = "exec", allow_single_file = True),
        "_mc": attr.label(default = "//bazel/rules/windows:mc", executable = True, cfg = "exec", allow_single_file = True),
        "_nls": attr.label(default = "//bazel/rules/windows:nls", cfg = "exec", allow_files = True),
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

    cc_info = _windres(ctx, src, syso_out, ctx.files.deps, args)
    return [DefaultInfo(files = depset([syso_out])), cc_info]

_win_resource = rule(
    implementation = _win_resource_impl,
    doc = "Compiles a .rc resource file into a .syso object via windres.",
    attrs = {
        "src": attr.label(mandatory = True, allow_single_file = [".rc"]),
        "deps": attr.label_list(allow_files = True),
        "defines": attr.string_dict(),
        "_llvm": attr.label(default = "//:llvm", executable = True, cfg = "exec", allow_single_file = True),
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
