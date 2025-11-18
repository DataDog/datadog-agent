""" Rule for building GNU Make from sources. """

load("@bazel_tools//tools/cpp:toolchain_utils.bzl", "find_cpp_toolchain")
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

def _make_tool_impl(ctx):
    cc_toolchain = find_cpp_toolchain(ctx)

    if "win" in os_name(ctx):
        build_str = "./build_w32.bat --without-guile"
        dist_dir = None

        if cc_toolchain.compiler == "mingw-gcc":
            build_str += " gcc"
            dist_dir = "GccRel"
        else:
            dist_dir = "WinRel"

        script = [
            build_str,
            "mkdir -p \"$$INSTALLDIR$$/bin\"",
            "cp -p ./{}/gnumake.exe \"$$INSTALLDIR$$/bin/make.exe\"".format(dist_dir),
        ]
    else:
        env = get_env_vars(ctx)
        flags_info = get_flags_info(ctx)
        tools_info = get_tools_info(ctx)

        ar_path = tools_info.cxx_linker_static
        frozen_arflags = flags_info.cxx_linker_static

        cc_path = tools_info.cc
        cflags = flags_info.cc
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
        arflags = [e for e in frozen_arflags]
        if absolute_ar == "libtool" or absolute_ar.endswith("/libtool"):
            arflags.append("-o")

        if os_name(ctx) == "macos":
            non_sysroot_ldflags += ["-undefined", "error"]

            # On macOS, remove "-lm".
            # During compilation, the ./configure script disables USE_SYSTEM_GLOB,
            # and chooses its own glob implementation (lib/glob.h, lib/glob.c).
            # all source files in lib/* are compiled to ./lib/libgnu.a
            # However, at link time, "-lm" appears before "-lgnu".
            # This linker commandline is like this: LINKER ... -lm -L./lib -o xxx ... -lgnu
            # So the system glob is linked instead, causing ABI conflicts.
            non_sysroot_ldflags = [x for x in non_sysroot_ldflags if x != "-lm"]

        configure_options = [
            "--without-guile",
            "--with-guile=no",
            "--disable-dependency-tracking",
            "--prefix=\"$$INSTALLDIR$$\"",
        ]

        install_cmd = ["./make install"]

        xcompile_options = detect_xcompile(ctx)
        if xcompile_options:
            configure_options.extend(xcompile_options)

            # We can't use make to install make when cross-compiling
            install_cmd = [
                "mkdir -p \"$$INSTALLDIR$$/bin\"",
                "cp -p make \"$$INSTALLDIR$$/bin/make\"",
            ]

        env.update({
            "AR": absolute_ar,
            "ARFLAGS": _join_flags_list(ctx.workspace_name, arflags),
            "CC": absolute_cc,
            "CFLAGS": _join_flags_list(ctx.workspace_name, non_sysroot_cflags),
            "LD": absolute_ld,
            "LDFLAGS": _join_flags_list(ctx.workspace_name, non_sysroot_ldflags),
        })

        configure_env = " ".join(["%s=\"%s\"" % (key, value) for key, value in env.items()])
        script = [
            "%s ./configure %s" % (configure_env, " ".join(configure_options)),
            "cat build.cfg",
            "./build.sh",
        ] + install_cmd

    return built_tool_rule_impl(
        ctx,
        script,
        ctx.actions.declare_directory("make"),
        "BootstrapGNUMake",
    )

make_tool = rule(
    doc = "Rule for building Make. Invokes configure script and make install.",
    attrs = FOREIGN_CC_BUILT_TOOLS_ATTRS,
    host_fragments = FOREIGN_CC_BUILT_TOOLS_HOST_FRAGMENTS,
    fragments = FOREIGN_CC_BUILT_TOOLS_FRAGMENTS,
    output_to_genfiles = True,
    implementation = _make_tool_impl,
    toolchains = [
        "@rules_foreign_cc//foreign_cc/private/framework:shell_toolchain",
        "@bazel_tools//tools/cpp:toolchain_type",
    ],
)

def _join_flags_list(workspace_name, flags):
    return " ".join([escape_dquote_bash(absolutize(workspace_name, flag)) for flag in flags])
