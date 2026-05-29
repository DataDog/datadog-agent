"""Hermetic MinGW-w64 cc_toolchain_config.

All tool paths are relative to the package containing the cc_toolchain target
(see @winlibs_mingw64//:winlibs.BUILD.bazel), so this rule has no host-path
attrs and works in sandboxed / remote-exec contexts.

WinLibs gcc.exe locates its sibling tools (as, ld, cc1, ...) via relative path
lookup against argv[0], which is why no PATH env_entry is needed once the full
WinLibs tree is staged as the cc_toolchain's `all_files`.
"""

load(
    "@bazel_tools//tools/cpp:cc_toolchain_config_lib.bzl",
    "action_config",
    "artifact_name_pattern",
    "feature",
    "flag_group",
    "flag_set",
    "tool",
    "tool_path",
)
load("@rules_cc//cc:action_names.bzl", "ACTION_NAMES")
load("@rules_cc//cc:defs.bzl", "CcToolchainConfigInfo", "cc_common")

_TOOL_NAMES = [
    "ar",
    "cpp",
    "gcc",
    "gcov",
    "ld",
    "nm",
    "objdump",
    "strip",
]

def _impl(ctx):
    gpp_tool = tool(path = "bin/g++.exe")
    gcc_tool = tool(path = "bin/gcc.exe")
    as_tool = tool(path = "bin/as.exe")
    ar_tool = tool(path = "bin/ar.exe")

    tool_paths = [
        tool_path(name = name, path = "bin/{}.exe".format(name))
        for name in _TOOL_NAMES
    ]

    default_feature = feature(
        name = "default_env_feature",
        enabled = True,
        flag_sets = [
            flag_set(
                actions = [ACTION_NAMES.cpp_compile, ACTION_NAMES.c_compile],
                flag_groups = [
                    flag_group(
                        flags = [
                            "-no-canonical-prefixes",
                            "-fno-canonical-system-headers",
                            "-Wno-builtin-macro-redefined",
                            "-D__DATE__=\"redacted\"",
                            "-D__TIMESTAMP__=\"redacted\"",
                            "-D__TIME__=\"redacted\"",
                        ],
                    ),
                ],
            ),
        ],
    )

    archiver_flags_feature = feature(
        name = "archiver_flags",
        enabled = True,
        flag_sets = [
            flag_set(
                actions = [ACTION_NAMES.cpp_link_static_library],
                flag_groups = [
                    flag_group(
                        flags = ["rcsD", "%{output_execpath}"],
                        # Needed for rules_foreign_cc which doesn't expose
                        # output_execpath; skip the group when absent.
                        expand_if_available = "output_execpath",
                    ),
                    flag_group(
                        iterate_over = "libraries_to_link",
                        flag_groups = [
                            flag_group(
                                flags = ["%{libraries_to_link.name}"],
                            ),
                        ],
                    ),
                ],
            ),
        ],
    )

    return [
        cc_common.create_cc_toolchain_config_info(
            ctx = ctx,
            features = [default_feature, archiver_flags_feature],
            toolchain_identifier = "mingw-toolchain",
            host_system_name = "nothing",
            target_system_name = "nothing",
            target_cpu = "x86_64",
            target_libc = "nothing",
            cc_target_os = "windows",
            compiler = "mingw-gcc",
            abi_version = "gcc-" + ctx.attr.gcc_version,
            abi_libc_version = "nothing",
            action_configs = [
                action_config(
                    action_name = ACTION_NAMES.c_compile,
                    enabled = True,
                    tools = [gcc_tool],
                ),
                action_config(
                    action_name = ACTION_NAMES.cpp_compile,
                    enabled = True,
                    tools = [gpp_tool],
                ),
                # .S files (assembly with C preprocessor) go through gcc.
                action_config(
                    action_name = ACTION_NAMES.preprocess_assemble,
                    enabled = True,
                    tools = [gcc_tool],
                ),
                # .s files (pure assembly) go straight to as.
                action_config(
                    action_name = ACTION_NAMES.assemble,
                    enabled = True,
                    tools = [as_tool],
                ),
                # g++ for linking covers both C and C++ link cases.
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
                action_config(
                    action_name = ACTION_NAMES.cpp_link_static_library,
                    enabled = True,
                    tools = [ar_tool],
                ),
            ],
            tool_paths = tool_paths,
            # Paths are relative to the cc_toolchain's package (the root of
            # @winlibs_mingw64//:), so rules_cc resolves them via crosstool_top
            # and never constructs a Label("@winlibs_mingw64//...") from inside
            # @@rules_cc+'s repo_mapping. Using %package(@winlibs_mingw64//)%
            # here trips rules_cc 0.2.18 on Bazel 9 with an invalid-Label error.
            cxx_builtin_include_directories = [
                "include",
                "lib/gcc/x86_64-w64-mingw32/" + ctx.attr.gcc_version + "/include-fixed",
                "lib/gcc/x86_64-w64-mingw32/" + ctx.attr.gcc_version + "/include",
                "lib/gcc/x86_64-w64-mingw32/" + ctx.attr.gcc_version + "/install-tools/include",
                "x86_64-w64-mingw32/include",
            ],
            artifact_name_patterns = [
                artifact_name_pattern(
                    category_name = "executable",
                    prefix = "",
                    extension = ".exe",
                ),
                artifact_name_pattern(
                    category_name = "dynamic_library",
                    prefix = "",
                    extension = ".dll",
                ),
                artifact_name_pattern(
                    category_name = "static_library",
                    prefix = "",
                    extension = ".a",
                ),
            ],
        ),
    ]

mingw_cc_toolchain_config = rule(
    implementation = _impl,
    attrs = {
        "gcc_version": attr.string(mandatory = True),
    },
    provides = [CcToolchainConfigInfo],
)
