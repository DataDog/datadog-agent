"""# [CMake](#cmake)

## Building CMake projects

- Build libraries/binaries with CMake from sources using cmake rule
- Use cmake targets in [cc_library][ccl], [cc_binary][ccb] targets as dependency
- Bazel [cc_toolchain][cct] parameters are used inside cmake build
- See full list of cmake arguments below 'example'
- Works on Ubuntu, Mac OS and Windows (*see special notes below in Windows section*) operating systems

**Example:**
(Please see full examples in ./examples)

The example for **Windows** is below, in the section 'Usage on Windows'.

- In `WORKSPACE.bazel`, we use a `http_archive` to download tarballs with the libraries we use.
- In `BUILD.bazel`, we instantiate a `cmake` rule which behaves similarly to a [cc_library][ccl], which can then be used in a C++ rule ([cc_binary][ccb] in this case).

In `WORKSPACE.bazel`, put

```python
workspace(name = "rules_foreign_cc_usage_example")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

# Rule repository, note that it's recommended to use a pinned commit to a released version of the rules
http_archive(
   name = "rules_foreign_cc",
   sha256 = "c2cdcf55ffaf49366725639e45dedd449b8c3fe22b54e31625eb80ce3a240f1e",
   strip_prefix = "rules_foreign_cc-0.1.0",
   url = "https://github.com/bazel-contrib/rules_foreign_cc/archive/0.1.0.zip",
)

load("@rules_foreign_cc//foreign_cc:repositories.bzl", "rules_foreign_cc_dependencies")

# This sets up some common toolchains for building targets. For more details, please see
# https://github.com/bazel-contrib/rules_foreign_cc/tree/main/docs#rules_foreign_cc_dependencies
rules_foreign_cc_dependencies()

_ALL_CONTENT = \"\"\"\\
filegroup(
    name = "all_srcs",
    srcs = glob(["**"]),
    visibility = ["//visibility:public"],
)
\"\"\"

# pcre source code repository
http_archive(
    name = "pcre",
    build_file_content = _ALL_CONTENT,
    strip_prefix = "pcre-8.43",
    urls = [
        "https://mirror.bazel.build/ftp.pcre.org/pub/pcre/pcre-8.43.tar.gz",
        "https://ftp.pcre.org/pub/pcre/pcre-8.43.tar.gz",
    ],
    sha256 = "0b8e7465dc5e98c757cc3650a20a7843ee4c3edf50aaf60bb33fd879690d2c73",
)
```

And in the `BUILD.bazel` file, put:

```python
load("@rules_foreign_cc//foreign_cc:defs.bzl", "cmake")

cmake(
    name = "pcre",
    cache_entries = {
        "CMAKE_C_FLAGS": "-fPIC",
    },
    lib_source = "@pcre//:all_srcs",
    out_static_libs = ["libpcre.a"],
)
```

then build as usual:

```bash
bazel build //:pcre
```

**Usage on Windows**

When using on Windows, you should start Bazel in MSYS2 shell, as the shell script inside cmake assumes this.
Also, you should explicitly specify **make commands and option to generate CMake crosstool file**.

The default generator for CMake will be detected automatically, or you can specify it explicitly.

**The tested generators:** Visual Studio 15, Ninja and NMake.
The extension `.lib` is assumed for the static libraries by default.

Example usage (see full example in `./examples/cmake_hello_world_lib`):
Example assumes that MS Visual Studio and Ninja are installed on the host machine, and Ninja bin directory is added to PATH.

```python
cmake(
    # expect to find ./lib/hello.lib as the result of the build
    name = "hello",
    # This option can be omitted
    generate_args = [
        "-G \\"Visual Studio 16 2019\\"",
        "-A Win64",
    ],
    lib_source = ":srcs",
)

cmake(
    name = "hello_ninja",
    # expect to find ./lib/hello.lib as the result of the build
    lib_name = "hello",
    # explicitly specify the generator
    generate_args = ["-GNinja"],
    lib_source = ":srcs",
)

cmake(
    name = "hello_nmake",
    # explicitly specify the generator
    generate_args = ["-G \\"NMake Makefiles\\""],
    lib_source = ":srcs",
    # expect to find ./lib/hello.lib as the result of the build
    out_static_libs = ["hello.lib"],
)
```

[ccb]: https://docs.bazel.build/versions/master/be/c-cpp.html#cc_binary
[ccl]: https://docs.bazel.build/versions/master/be/c-cpp.html#cc_library
[cct]: https://docs.bazel.build/versions/master/be/c-cpp.html#cc_toolchain
"""

load("@rules_cc//cc:defs.bzl", "CcInfo")
load(
    "//foreign_cc/private:cc_toolchain_util.bzl",
    "get_flags_info",
    "get_tools_info",
    "is_debug_mode",
)
load("//foreign_cc/private:cmake_script.bzl", "create_cmake_script")
load("//foreign_cc/private:detect_root.bzl", "detect_root")
load(
    "//foreign_cc/private:framework.bzl",
    "CC_EXTERNAL_RULE_ATTRIBUTES",
    "CC_EXTERNAL_RULE_FRAGMENTS",
    "cc_external_rule_impl",
    "create_attrs",
    "expand_locations_and_make_variables",
)
load("//foreign_cc/private:transitions.bzl", "foreign_cc_rule_variant")
load(
    "//foreign_cc/private/framework:platform.bzl",
    "arch_name",
    "os_name",
    "target_arch_name",
    "target_os_name",
)
load(
    "//toolchains/native_tools:tool_access.bzl",
    "get_cmake_data",
    "get_m4_data",
    "get_make_data",
    "get_ninja_data",
    "get_pkgconfig_data",
)

def _cmake_impl(ctx):
    cmake_data = get_cmake_data(ctx)
    pkgconfig_data = get_pkgconfig_data(ctx)
    m4_data = get_m4_data(ctx)

    tools_data = [cmake_data, pkgconfig_data, m4_data]

    env = dict(ctx.attr.env)

    generator, generate_args = _get_generator_target(ctx)
    if "Unix Makefiles" == generator:
        make_data = get_make_data(ctx)
        tools_data.append(make_data)
        generate_args.append("-DCMAKE_MAKE_PROGRAM={}".format(make_data.path))
    elif "Ninja" in generator:
        ninja_data = get_ninja_data(ctx)
        tools_data.append(ninja_data)
        generate_args.append("-DCMAKE_MAKE_PROGRAM={}".format(ninja_data.path))

    attrs = create_attrs(
        ctx.attr,
        env = env,
        generator = generator,
        generate_args = generate_args,
        configure_name = "CMake",
        create_configure_script = _create_configure_script,
        tools_data = tools_data,
        cmake_path = cmake_data.path,
    )

    return cc_external_rule_impl(ctx, attrs)

def _create_configure_script(configureParameters):
    ctx = configureParameters.ctx
    attrs = configureParameters.attrs
    inputs = configureParameters.inputs

    root = detect_root(ctx.attr.lib_source)
    if len(ctx.attr.working_directory) > 0:
        root = root + "/" + ctx.attr.working_directory

    tools = get_tools_info(ctx)

    # CMake will replace <TARGET> with the actual output file
    flags = get_flags_info(ctx, "<TARGET>")
    no_toolchain_file = ctx.attr.cache_entries.get("CMAKE_TOOLCHAIN_FILE") or not ctx.attr.generate_crosstool_file

    cmake_commands = []

    default_configuration = "Debug" if is_debug_mode(ctx) else "Release"
    configuration = default_configuration if not ctx.attr.configuration else ctx.attr.configuration

    data = ctx.attr.data + ctx.attr.build_data

    # TODO: The following attributes are deprecated. Remove
    data += ctx.attr.tools_deps

    # Generate a list of arguments for cmake's build command
    build_args = " ".join([
        expand_locations_and_make_variables(ctx, arg, "build_args", data)
        for arg in ctx.attr.build_args
    ])

    # Generate commands for all the targets, ensuring there's
    # always at least 1 call to the default target.
    for target in ctx.attr.targets or [""]:
        # There's no need to use the `--target` argument for an empty/"all" target
        if target:
            target = "--target '{}'".format(target)

        # Note that even though directory is always passed, the
        # following arguments can take precedence.
        cmake_commands.append("{cmake} --build {dir} --config {config} {target} {args}".format(
            cmake = attrs.cmake_path,
            dir = ".",
            args = build_args,
            target = target,
            config = configuration,
        ))

    if ctx.attr.install:
        # Generate a list of arguments for cmake's install command
        install_args = " ".join([
            expand_locations_and_make_variables(ctx, arg, "install_args", data)
            for arg in ctx.attr.install_args
        ])

        cmake_commands.append("{cmake} --install {dir} --config {config} {args}".format(
            cmake = attrs.cmake_path,
            dir = ".",
            args = install_args,
            config = configuration,
        ))

    prefix = expand_locations_and_make_variables(ctx, attrs.tool_prefix, "tool_prefix", data) if attrs.tool_prefix else ""

    configure_script = create_cmake_script(
        workspace_name = ctx.workspace_name,
        current_label = ctx.label,
        target_os = target_os_name(ctx),
        target_arch = target_arch_name(ctx),
        host_os = os_name(ctx),
        host_arch = arch_name(ctx),
        generator = attrs.generator,
        cmake_path = attrs.cmake_path,
        tools = tools,
        flags = flags,
        install_prefix = "$$INSTALLDIR$$",
        root = root,
        no_toolchain_file = no_toolchain_file,
        user_cache = expand_locations_and_make_variables(ctx, ctx.attr.cache_entries, "cache_entries", data),
        user_env = expand_locations_and_make_variables(ctx, ctx.attr.env, "env", data),
        options = attrs.generate_args,
        cmake_commands = cmake_commands,
        cmake_prefix = prefix,
        include_dirs = inputs.include_dirs,
        is_debug_mode = is_debug_mode(ctx),
        ext_build_dirs = inputs.ext_build_dirs,
    )
    return configure_script

def _get_generator_target(ctx):
    """Parse the genrator arguments for a generator declaration

    If none is found, a default will be chosen

    Args:
        ctx (ctx): The rule's context object

    Returns:
        tuple: (str, list) the generator and a list of arguments with the generator arg removed
    """
    known_generators = [
        "Borland Makefiles",
        "Green Hills MULTI",
        "MinGW Makefiles",
        "MSYS Makefiles",
        "Ninja Multi-Config",
        "Ninja",
        "NMake Makefiles JOM",
        "NMake Makefiles",
        "Unix Makefiles",
        "Visual Studio 10 2010",
        "Visual Studio 11 2012",
        "Visual Studio 12 2013",
        "Visual Studio 14 2015",
        "Visual Studio 15 2017",
        "Visual Studio 16 2019",
        "Visual Studio 17 2022",
        "Visual Studio 9 2008",
        "Watcom WMake",
        "Xcode",
    ]

    generator = None

    generator_definitions = []

    # Create a mutable list
    generate_args = list(ctx.attr.generate_args)
    for arg in generate_args:
        if arg.startswith("-G"):
            generator_definitions.append(arg)
            break

    if len(generator_definitions) > 1:
        fail("Please specify no more than 1 generator argument. Arguments found: {}".format(generator_definitions))

    for definition in generator_definitions:
        generator = definition[2:]
        generator = generator.strip(" =\"'")

        # Remove the argument so it's not passed twice to the cmake command
        # See create_cmake_script for more details
        generate_args.remove(definition)

    if not generator:
        execution_os_name = os_name(ctx)
        if "win" in execution_os_name:
            generator = "Ninja"
        elif "macos" in execution_os_name:
            generator = "Unix Makefiles"
        elif "linux" in execution_os_name:
            generator = "Unix Makefiles"
        else:
            fail("No generator set and no default is known. Please set the cmake `generator` attribute")

    # Sanity check
    for gen in known_generators:
        if generator.startswith(gen):
            return generator, generate_args

    fail("`{}` is not a known generator".format(generator))

def _attrs():
    attrs = dict(CC_EXTERNAL_RULE_ATTRIBUTES)
    attrs.update({
        "build_args": attr.string_list(
            doc = "Arguments for the CMake build command",
            mandatory = False,
        ),
        "cache_entries": attr.string_dict(
            doc = (
                "CMake cache entries to initialize (they will be passed with `-Dkey=value`) " +
                "Values, defined by the toolchain, will be joined with the values, passed here. " +
                "(Toolchain values come first)"
            ),
            mandatory = False,
            default = {},
        ),
        "configuration": attr.string(
            doc = (
                "Override the `cmake --build` and `cmake --install` `--config` configuration. " +
                "If left empty, the value of this arg will be determined by the COMPILATION_MODE env var: " +
                "dbg will set `--config Debug` and all other modes will set --config Release."
            ),
            mandatory = False,
            default = "",
        ),
        "generate_args": attr.string_list(
            doc = (
                "Arguments for CMake's generate command. Arguments should be passed as key/value pairs. eg: " +
                "`[\"-G Ninja\", \"--debug-output\", \"-DFOO=bar\"]`. Note that unless a generator (`-G`) argument " +
                "is provided, the default generators are [Unix Makefiles](https://cmake.org/cmake/help/latest/generator/Unix%20Makefiles.html) " +
                "for Linux and MacOS and [Ninja](https://cmake.org/cmake/help/latest/generator/Ninja.html) for " +
                "Windows."
            ),
            mandatory = False,
            default = [],
        ),
        "generate_crosstool_file": attr.bool(
            doc = (
                "When True, CMake crosstool file will be generated from the toolchain values, " +
                "provided cache-entries and env_vars (some values will still be passed as `-Dkey=value` " +
                "and environment variables). If `CMAKE_TOOLCHAIN_FILE` cache entry is passed, " +
                "specified crosstool file will be used When using this option to cross-compile, " +
                "it is required to specify `CMAKE_SYSTEM_NAME` in the cache_entries"
            ),
            mandatory = False,
            default = True,
        ),
        "install": attr.bool(
            doc = "If True, the `cmake --install` comand will be performed after a build",
            default = True,
        ),
        "install_args": attr.string_list(
            doc = "Arguments for the CMake install command",
            mandatory = False,
        ),
        "working_directory": attr.string(
            doc = (
                "Working directory, with the main CMakeLists.txt " +
                "(otherwise, the top directory of the lib_source label files is used.)"
            ),
            mandatory = False,
            default = "",
        ),
    })
    return attrs

cmake = rule(
    doc = "Rule for building external library with CMake.",
    attrs = _attrs(),
    fragments = CC_EXTERNAL_RULE_FRAGMENTS,
    output_to_genfiles = True,
    implementation = _cmake_impl,
    toolchains = [
        "@rules_foreign_cc//toolchains:cmake_toolchain",
        "@rules_foreign_cc//toolchains:ninja_toolchain",
        "@rules_foreign_cc//toolchains:make_toolchain",
        "@rules_foreign_cc//toolchains:m4_toolchain",
        "@rules_foreign_cc//toolchains:pkgconfig_toolchain",
        "@rules_foreign_cc//foreign_cc/private/framework:shell_toolchain",
        "@bazel_tools//tools/cpp:toolchain_type",
    ],
    provides = [CcInfo],
)

def cmake_variant(name, toolchain, **kwargs):
    """ Wrapper macro around the cmake() rule to force usage of the given make variant toolchain.

    Args:
        name: The target name
        toolchain: The desired make variant toolchain to use, e.g. @rules_foreign_cc//toolchains:preinstalled_nmake_toolchain
        **kwargs: Remaining keyword arguments
    """
    foreign_cc_rule_variant(
        name = name,
        rule = cmake,
        toolchain = toolchain,
        **kwargs
    )
