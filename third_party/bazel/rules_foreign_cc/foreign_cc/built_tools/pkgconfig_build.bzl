""" Rule for building pkg-config from source. """

load("//foreign_cc:defs.bzl", "make_variant", "runnable_binary")
load(
    "//foreign_cc/built_tools/private:built_tools_framework.bzl",
    "FOREIGN_CC_BUILT_TOOLS_ATTRS",
    "FOREIGN_CC_BUILT_TOOLS_FRAGMENTS",
    "FOREIGN_CC_BUILT_TOOLS_HOST_FRAGMENTS",
    "absolutize",
    "built_tool_rule_impl",
    "extract_non_sysroot_flags",
    "extract_sysroot_flags",
)
load(
    "//foreign_cc/private:cc_toolchain_util.bzl",
    "get_env_vars",
    "get_flags_info",
    "get_tools_info",
)
load("//foreign_cc/private:detect_xcompile.bzl", "detect_xcompile")
load(
    "//foreign_cc/private/framework:helpers.bzl",
    "escape_dquote_bash",
)
load("//foreign_cc/private/framework:platform.bzl", "os_name")
load("//toolchains/native_tools:tool_access.bzl", "get_make_data")

def _pkgconfig_tool_impl(ctx):
    env = get_env_vars(ctx)
    flags_info = get_flags_info(ctx)
    tools_info = get_tools_info(ctx)

    ar_path = tools_info.cxx_linker_static
    frozen_arflags = flags_info.cxx_linker_static

    cc_path = tools_info.cc
    cflags = flags_info.cc + ["-Wno-int-conversion"]  # Fix building with clang 15+
    sysroot_cflags = extract_sysroot_flags(cflags)
    non_sysroot_cflags = extract_non_sysroot_flags(cflags)

    ld_path = tools_info.cxx_linker_executable
    ldflags = flags_info.cxx_linker_executable
    sysroot_ldflags = extract_sysroot_flags(ldflags)
    non_sysroot_ldflags = extract_non_sysroot_flags(ldflags)

    # Make's build script does not forward CFLAGS to all compiler and linker
    # invocations, so we append --sysroot flags directly to CC and LD.
    absolute_cc = absolutize(ctx.workspace_name, cc_path, True)
    if sysroot_cflags:
        absolute_cc += " " + _join_flags_list(ctx.workspace_name, sysroot_cflags)
    absolute_ld = absolutize(ctx.workspace_name, ld_path, True)
    if sysroot_ldflags:
        absolute_ld += " " + _join_flags_list(ctx.workspace_name, sysroot_ldflags)

    # If libtool is used as AR, the output file has to be prefixed with
    # "-o". Since the make Makefile only uses ar-style invocations, the
    # output file always comes first and we can append this argument to the
    # flags list.
    absolute_ar = absolutize(ctx.workspace_name, ar_path, True)

    if os_name(ctx) == "macos":
        absolute_ar = ""
        non_sysroot_ldflags += ["-undefined", "error"]

    arflags = [e for e in frozen_arflags]
    if absolute_ar == "libtool" or absolute_ar.endswith("/libtool"):
        arflags.append("-o")

    make_data = get_make_data(ctx)

    configure_options = [
        "--with-internal-glib",
        "--prefix=\"$$INSTALLDIR$$\"",
    ]

    xcompile_options = detect_xcompile(ctx)
    if xcompile_options:
        configure_options.extend(xcompile_options)

    env.update({
        "AR": absolute_ar,
        "ARFLAGS": _join_flags_list(ctx.workspace_name, arflags),
        "CC": absolute_cc,
        "CFLAGS": _join_flags_list(ctx.workspace_name, non_sysroot_cflags),
        "LD": absolute_ld,
        "LDFLAGS": _join_flags_list(ctx.workspace_name, non_sysroot_ldflags),
        "MAKE": make_data.path,
    })

    configure_env = " ".join(["%s=\"%s\"" % (key, value) for key, value in env.items()])
    script = [
        "%s ./configure %s" % (configure_env, " ".join(configure_options)),
        "\"%s\"" % make_data.path,
        "\"%s\" install" % make_data.path,
    ]

    if make_data.target:
        additional_tools = depset(transitive = [make_data.target.files])
    else:
        additional_tools = depset()

    return built_tool_rule_impl(
        ctx,
        script,
        ctx.actions.declare_directory("pkgconfig"),
        "BootstrapPkgConfig",
        additional_tools,
    )

pkgconfig_tool_unix = rule(
    doc = "Rule for building pkgconfig on Unix operating systems",
    attrs = FOREIGN_CC_BUILT_TOOLS_ATTRS,
    host_fragments = FOREIGN_CC_BUILT_TOOLS_HOST_FRAGMENTS,
    fragments = FOREIGN_CC_BUILT_TOOLS_FRAGMENTS,
    output_to_genfiles = True,
    implementation = _pkgconfig_tool_impl,
    toolchains = [
        "@rules_foreign_cc//foreign_cc/private/framework:shell_toolchain",
        "@rules_foreign_cc//toolchains:make_toolchain",
        "@bazel_tools//tools/cpp:toolchain_type",
    ],
)

def pkgconfig_tool(name, srcs, **kwargs):
    """
    Macro that provides targets for building pkg-config from source

    Args:
        name: The target name
        srcs: The pkg-config source files
        **kwargs: Remaining keyword arguments
    """
    tags = ["manual"] + kwargs.pop("tags", [])

    native.config_setting(
        name = "msvc_compiler",
        flag_values = {
            "@bazel_tools//tools/cpp:compiler": "msvc-cl",
        },
    )

    native.alias(
        name = name,
        actual = select({
            ":msvc_compiler": "{}_msvc".format(name),
            "//conditions:default": "{}_default".format(name),
        }),
    )

    pkgconfig_tool_unix(
        name = "{}_default".format(name),
        srcs = srcs,
        tags = tags,
        **kwargs
    )

    make_variant(
        name = "{}_msvc_build".format(name),
        lib_source = srcs,
        args = [
            "-f Makefile.vc",
            "CFG=release",
            "GLIB_PREFIX=\"$$EXT_BUILD_ROOT/external/glib_dev\"",
        ],
        out_binaries = ["pkg-config.exe"],
        env = {"INCLUDE": "$$EXT_BUILD_ROOT/external/glib_src"},
        out_static_libs = [],
        out_shared_libs = [],
        deps = [
            "@glib_dev",
            "@glib_src//:msvc_hdr",
            "@gettext_runtime",
        ],
        postfix_script = select({
            "@platforms//os:windows": "cp release/x64/pkg-config.exe $$INSTALLDIR/bin",
            "//conditions:default": "",
        }),
        toolchain = str(Label("//toolchains:preinstalled_nmake_toolchain")),
        tags = tags,
        **kwargs
    )

    runnable_binary(
        name = "{}_msvc".format(name),
        binary = "pkg-config",
        foreign_cc_target = "{}_msvc_build".format(name),
        # Tools like CMake and Meson search for "pkg-config" on the PATH
        match_binary_name = True,
        tags = tags,
    )

def _join_flags_list(workspace_name, flags):
    return " ".join([escape_dquote_bash(absolutize(workspace_name, flag)) for flag in flags])
