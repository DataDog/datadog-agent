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

    gcc_tool = tool(
        path = ctx.attr.MINGW_PATH + "/bin/gcc",
    )

    # For assembly files, GCC can act as the assembler (preprocesses and assembles .S files)
    as_tool = tool(
        path = ctx.attr.MINGW_PATH + "/bin/as",
    )

    # For creating static libraries (.a files)
    ar_tool = tool(
        path = ctx.attr.MINGW_PATH + "/bin/ar",
    )

    tool_paths = []

    for mingw_tool in tools:
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
        env_sets = [
            env_set(
                actions = [
                    ACTION_NAMES.c_compile,
                    ACTION_NAMES.cpp_compile,
                    ACTION_NAMES.assemble,
                    ACTION_NAMES.preprocess_assemble,
                    ACTION_NAMES.cpp_link_executable,
                    ACTION_NAMES.cpp_link_dynamic_library,
                    ACTION_NAMES.cpp_link_static_library,
                ],
                env_entries = [
                    env_entry("PATH", "{}/usr/bin;{}/bin".format(ctx.attr.MSYS2_PATH, ctx.attr.MINGW_PATH)),
                ],
            ),
        ],
    )

    # Feature for archiver flags (ar)
    archiver_flags_feature = feature(
        name = "archiver_flags",
        enabled = True,
        flag_sets = [
            flag_set(
                actions = [ACTION_NAMES.cpp_link_static_library],
                flag_groups = [
                    flag_group(
                        flags = ["rcsD", "%{output_execpath}"],
                        # This is needed to make rules_foreign_cc work
                        # as it cannot find output_execpath. This way we
                        # don't expand this flag group if variable is not available.
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
            abi_version = "gcc-" + ctx.attr.GCC_VERSION,
            abi_libc_version = "nothing",
            action_configs = [
                # C compilation - use gcc
                action_config(
                    action_name = ACTION_NAMES.c_compile,
                    enabled = True,
                    tools = [gcc_tool],
                ),
                # C++ compilation - use g++
                action_config(
                    action_name = ACTION_NAMES.cpp_compile,
                    enabled = True,
                    tools = [gpp_tool],
                ),
                # Assembly actions
                # preprocess_assemble: for .S files (assembly with C preprocessor)
                action_config(
                    action_name = ACTION_NAMES.preprocess_assemble,
                    enabled = True,
                    tools = [gcc_tool],  # GCC can preprocess and assemble .S files
                ),
                # assemble: for .s files (pure assembly, no preprocessing)
                action_config(
                    action_name = ACTION_NAMES.assemble,
                    enabled = True,
                    tools = [as_tool],  # Use 'as' directly for pure assembly
                ),
                # Linking actions (shared between C and C++)
                # For C projects, gcc is typically used; for C++ projects, g++ is used
                # We'll use g++ for linking to handle both cases properly
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
                # Static library archiving - use ar (archiver)
                action_config(
                    action_name = ACTION_NAMES.cpp_link_static_library,
                    enabled = True,
                    tools = [ar_tool],
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
        "MSYS2_PATH": attr.string(mandatory = True),
        "MINGW_PATH": attr.string(mandatory = True),
        "GCC_VERSION": attr.string(mandatory = True),
    },
    provides = [CcToolchainConfigInfo],
)
