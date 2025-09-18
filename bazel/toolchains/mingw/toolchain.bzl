load(
    "@bazel_tools//tools/cpp:cc_toolchain_config_lib.bzl",
    "action_config",
    "artifact_name_pattern",
    "tool",
    "tool_path",
)
load("@rules_cc//cc:action_names.bzl", "ACTION_NAMES")

def _impl(ctx):
    out = ctx.actions.declare_file(ctx.label.name)
    ctx.actions.write(out, "executable")

    # path to external MINGW Compiler (e.g: "C:/toolchains/TDM-GCC-64")
    if ctx.var.get("MINGW_PATH"):
        MINGW_PATH = ctx.var.get("MINGW_PATH")
    else:
        MINGW_PATH = "C:/tools/msys64/mingw64"

    # MINGW Compiler Version (e.g: "10.3.0")
    if ctx.var.get("GCC_VERSION"):
        GCC_VERSION = ctx.var.get("GCC_VERSION")
    else:
        GCC_VERSION = "14.2.0"

    gpp_tool = tool(
        path = MINGW_PATH + "/bin/g++",
    )

    return [
        cc_common.create_cc_toolchain_config_info(
            ctx = ctx,
            toolchain_identifier = "mingw-toolchain",
            host_system_name = "nothing",
            target_system_name = "nothing",
            target_cpu = "x86_64",
            target_libc = "nothing",
            cc_target_os = "windows",
            compiler = "mingw-gcc",
            abi_version = "gcc-" + GCC_VERSION,
            abi_libc_version = "nothing",
            action_configs = [
                # TODO: This requires further research on how to do properly, even
                # though for now it does what we need: set g++ for appropriate c++ targets
                action_config(
                    action_name = ACTION_NAMES.cpp_compile,
                    enabled = True,
                    tools = [gpp_tool],
                ),
                action_config(
                    action_name = ACTION_NAMES.cpp_link_executable,
                    enabled = True,
                    tools = [gpp_tool],
                ),
                action_config(
                    action_name = ACTION_NAMES.cpp_link_dynamic_library,
                    enabled = True,
                    tools = [gpp_tool],
                ),
            ],
            tool_paths = [
                tool_path(
                    name = "ar",
                    path = MINGW_PATH + "/bin/ar",
                ),
                tool_path(
                    name = "cpp",
                    path = MINGW_PATH + "/bin/cpp",
                ),
                tool_path(
                    name = "gcc",
                    path = MINGW_PATH + "/bin/gcc",
                ),
                tool_path(
                    name = "gcov",
                    path = MINGW_PATH + "/bin/gcov",
                ),
                tool_path(
                    name = "ld",
                    path = MINGW_PATH + "/bin/ld",
                ),
                tool_path(
                    name = "nm",
                    path = MINGW_PATH + "/bin/nm",
                ),
                tool_path(
                    name = "objdump",
                    path = MINGW_PATH + "/bin/objdump",
                ),
                tool_path(
                    name = "strip",
                    path = MINGW_PATH + "/bin/strip",
                ),
            ],
            cxx_builtin_include_directories = [
                MINGW_PATH + "/include",
                MINGW_PATH + "/lib/gcc/x86_64-w64-mingw32/" + GCC_VERSION + "/include-fixed",
                MINGW_PATH + "/lib/gcc/x86_64-w64-mingw32/" + GCC_VERSION + "/include",
                MINGW_PATH + "/lib/gcc/x86_64-w64-mingw32/" + GCC_VERSION + "/install-tools/include",
                MINGW_PATH + "/x86_64-w64-mingw32/include",
            ],
            artifact_name_patterns = [
                artifact_name_pattern(
                    category_name = "executable",
                    prefix = "",
                    extension = ".exe",
                ),
            ],
        ),
        DefaultInfo(
            executable = out,
        ),
    ]

mingw_cc_toolchain_config = rule(
    implementation = _impl,
    provides = [CcToolchainConfigInfo],
    executable = True,
)
