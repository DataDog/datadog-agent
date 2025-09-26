load(
    "@bazel_tools//tools/cpp:cc_toolchain_config_lib.bzl",
    "action_config",
    "artifact_name_pattern",
    "env_entry",
    "env_set",
    "feature",
    "flag_group",
    "flag_set",
    "tool",
    "tool_path",
)
load("@rules_cc//cc:action_names.bzl", "ACTION_NAMES")
load("@rules_cc//cc/common:cc_common.bzl", "cc_common")

def _impl(ctx):
    tools = [
        "g++",
        "ar",
        "cpp",
        "gcc",
        "gcov",
        "ld",
        "nm",
        "objdump",
        "strip",
    ]

    gpp_tool = tool(
        path = ctx.attr.MINGW_PATH + "/bin/g++",
    )

    tool_paths = []

    for mingw_tool in tools:
        if mingw_tool != "g++":
            tool_paths.append(tool_path(name = mingw_tool, path = "{}/bin/{}".format(ctx.attr.MINGW_PATH, mingw_tool)))

    default_feature = feature(
        name = "default_env_feature",
        enabled = True,
        flag_sets = [
            flag_set(
                actions = [ACTION_NAMES.cpp_compile, ACTION_NAMES.c_compile],
                flag_groups = [
                    flag_group(
                        flags = [
                            "-Wno-builtin-macro-redefined",
                            "-D__DATE__=\"redacted\"",
                            "-D__TIMESTAMP__=\"redacted\"",
                            "-D__TIME__=\"redacted\"",
                        ],
                    ),
                ],
            ),
        ],
        env_sets = [
            env_set(
                actions = [ACTION_NAMES.cpp_compile, ACTION_NAMES.c_compile, ACTION_NAMES.cpp_link_executable, ACTION_NAMES.cpp_link_dynamic_library],
                env_entries = [env_entry("PATH", "{}/bin".format(ctx.attr.MINGW_PATH))],
            ),
        ],
    )

    return [
        cc_common.create_cc_toolchain_config_info(
            ctx = ctx,
            features = [default_feature],
            toolchain_identifier = "mingw-toolchain",
            host_system_name = "nothing",
            target_system_name = "nothing",
            target_cpu = "x86_64",
            target_libc = "nothing",
            cc_target_os = "windows",
            compiler = "mingw-gcc",
            abi_version = "gcc-" + ctx.attr.GCC_VERSION,
            abi_libc_version = "nothing",
            action_configs = [
                # TODO: This requires further research on how to do properly, even
                # though for now it does what we need: set g++ for appropriate c++ targets
                action_config(
                    action_name = ACTION_NAMES.cpp_compile + ACTION_NAMES.c_compile,
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
            tool_paths = tool_paths,
            cxx_builtin_include_directories = [
                ctx.attr.MINGW_PATH + "/include",
                ctx.attr.MINGW_PATH + "/lib/gcc/x86_64-w64-mingw32/" + ctx.attr.GCC_VERSION + "/include-fixed",
                ctx.attr.MINGW_PATH + "/lib/gcc/x86_64-w64-mingw32/" + ctx.attr.GCC_VERSION + "/include",
                ctx.attr.MINGW_PATH + "/lib/gcc/x86_64-w64-mingw32/" + ctx.attr.GCC_VERSION + "/install-tools/include",
                ctx.attr.MINGW_PATH + "/x86_64-w64-mingw32/include",
            ],
            artifact_name_patterns = [
                artifact_name_pattern(
                    category_name = "executable",
                    prefix = "",
                    extension = ".exe",
                ),
                artifact_name_pattern(
                    category_name = "dynamic_library",
                    prefix = "lib",
                    extension = ".dll",
                ),
            ],
        ),
    ]

mingw_cc_toolchain_config = rule(
    implementation = _impl,
    attrs = {
        "MINGW_PATH": attr.string(mandatory = True),
        "GCC_VERSION": attr.string(mandatory = True),
    },
    provides = [CcToolchainConfigInfo],
)
