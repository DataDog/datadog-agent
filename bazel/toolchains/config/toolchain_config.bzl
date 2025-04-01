# Defines the C++ settings that tell Bazel precisely how to construct C++
# commands. This is unique to C++ toolchains: other languages don't require
# anything like this.
#
# See
# https://bazel.build/docs/cc-toolchain-config-reference
# for all the gory details.
#
# This file is more about C++-specific toolchain configuration than how to
# declare toolchains and match them to platforms. It's important if you want to
# write your own custom C++ toolchains. But if you want to write toolchains for
# other languages or figure out how to select toolchains for custom CPU types,
# OSes, etc., the BUILD file is much more interesting.

load("@bazel_tools//tools/build_defs/cc:action_names.bzl", "ACTION_NAMES")
load(
    "@bazel_tools//tools/cpp:cc_toolchain_config_lib.bzl",
    "artifact_name_pattern",
    "tool_path",
    "feature",
    "flag_set",
    "flag_group",
)

def _impl(ctx):
    out = ctx.actions.declare_file(ctx.label.name)
    ctx.actions.write(out, "executable")

    GCC_VERSION = ctx.attr.gcc_version
    TOOLCHAIN_ARCH = ctx.attr.arch
    TOOLCHAIN_PATH = ctx.attr.path
    TRIPLET = TOOLCHAIN_ARCH + "-unknown-linux-gnu"

    all_compile_actions = [
        ACTION_NAMES.c_compile,
        ACTION_NAMES.cpp_compile,
        ACTION_NAMES.cpp_module_codegen,
        ACTION_NAMES.cpp_module_compile,
    ]

    all_link_actions = [
        ACTION_NAMES.cpp_link_executable,
        ACTION_NAMES.cpp_link_dynamic_library,
        ACTION_NAMES.cpp_link_nodeps_dynamic_library,
    ]

    pic_flags = feature(
        name = "pic",
        enabled = True,
        flag_sets = [
            flag_set(
                actions = all_compile_actions + all_link_actions,
                flag_groups = [
                    flag_group(
                        flags = ["-fPIC"],
                    ),
                ],
            ),
        ],
    )

    tools = ["ar", "cpp", "gcc", "g++", "gcov", "ld", "nm", "objdump", "strip"]
    tools_path = [tool_path(name = t, path = "{}/bin/{}-{}".format(TOOLCHAIN_PATH, TRIPLET, t)) for t in tools]

    return [
        cc_common.create_cc_toolchain_config_info(
            ctx = ctx,
            toolchain_identifier = "glibc-toolchain",
            target_cpu = TOOLCHAIN_ARCH,
            cc_target_os = "linux",
            compiler = "gcc",
            abi_version = "gcc-" + GCC_VERSION,
            abi_libc_version = "nothing",
            features = [
                pic_flags,
            ],
            tool_paths = tools_path,
            cxx_builtin_include_directories = [
                TOOLCHAIN_PATH + "/include",
                TOOLCHAIN_PATH + "/lib/gcc/" + TOOLCHAIN_ARCH + "-unknown-linux-gnu/" + GCC_VERSION + "/include-fixed",
                TOOLCHAIN_PATH + "/lib/gcc/" + TOOLCHAIN_ARCH + "-unknown-linux-gnu/" + GCC_VERSION + "/include",
                TOOLCHAIN_PATH + "/lib/gcc/" + TOOLCHAIN_ARCH + "-unknown-linux-gnu/" + GCC_VERSION + "/install-tools/include",
                TOOLCHAIN_PATH + "/" + TOOLCHAIN_ARCH + "-unknown-linux-gnu/include",
                "{}/{}/sysroot/usr/include/".format(TOOLCHAIN_PATH, TRIPLET),
            ],
        ),
        DefaultInfo(
            executable = out,
        ),
    ]

glibc_cc_toolchain_config = rule(
    implementation = _impl,
    provides = [CcToolchainConfigInfo],
    executable = True,
    attrs = {
        "arch": attr.string(mandatory=True),
        "gcc_version": attr.string(mandatory=True),
        "path": attr.string(mandatory=True),
    },
)
