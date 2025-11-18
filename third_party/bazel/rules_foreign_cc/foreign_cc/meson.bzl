"""A rule for building projects using the [Meson](https://mesonbuild.com/) build system"""

load("@rules_cc//cc:defs.bzl", "CcInfo")
load("//foreign_cc:utils.bzl", "full_label")
load("//foreign_cc/built_tools:meson_build.bzl", "meson_tool")
load(
    "//foreign_cc/private:cc_toolchain_util.bzl",
    "absolutize_path_in_str",
    "get_flags_info",
    "get_tools_info",
)
load(
    "//foreign_cc/private:detect_root.bzl",
    "detect_root",
)
load(
    "//foreign_cc/private:framework.bzl",
    "CC_EXTERNAL_RULE_ATTRIBUTES",
    "CC_EXTERNAL_RULE_FRAGMENTS",
    "cc_external_rule_impl",
    "create_attrs",
    "expand_locations_and_make_variables",
)
load("//foreign_cc/private:make_script.bzl", "pkgconfig_script")
load("//foreign_cc/private:transitions.bzl", "foreign_cc_rule_variant")
load("//toolchains/native_tools:native_tools_toolchain.bzl", "native_tool_toolchain")
load("//toolchains/native_tools:tool_access.bzl", "get_cmake_data", "get_meson_data", "get_ninja_data", "get_pkgconfig_data")

def _meson_impl(ctx):
    """The implementation of the `meson` rule

    Args:
        ctx (ctx): The rule's context object

    Returns:
        list: A list of providers. See `cc_external_rule_impl`
    """

    meson_data = get_meson_data(ctx)
    cmake_data = get_cmake_data(ctx)
    ninja_data = get_ninja_data(ctx)
    pkg_config_data = get_pkgconfig_data(ctx)

    tools_data = [meson_data, cmake_data, ninja_data, pkg_config_data]

    attrs = create_attrs(
        ctx.attr,
        configure_name = "Meson",
        create_configure_script = _create_meson_script,
        tools_data = tools_data,
        meson_path = meson_data.path,
        cmake_path = cmake_data.path,
        ninja_path = ninja_data.path,
        pkg_config_path = pkg_config_data.path,
    )
    return cc_external_rule_impl(ctx, attrs)

def _create_meson_script(configureParameters):
    """Creates the bash commands for invoking commands to build meson projects

    Args:
        configureParameters (struct): See `ConfigureParameters`

    Returns:
        str: A string representing a section of a bash script
    """
    ctx = configureParameters.ctx
    attrs = configureParameters.attrs
    inputs = configureParameters.inputs

    tools = get_tools_info(ctx)
    flags = get_flags_info(ctx)
    script = pkgconfig_script(inputs.ext_build_dirs)

    # CFLAGS and CXXFLAGS are also set in foreign_cc/private/cmake_script.bzl, so that meson
    # can use the intended tools.
    # However, they are split by meson on whitespace. For Windows it's common to have spaces in path
    # https://github.com/mesonbuild/meson/issues/3565
    # Skip setting them in this case.
    if " " not in tools.cc:
        script.append("##export_var## CC {}".format(_absolutize(ctx.workspace_name, tools.cc)))
    if " " not in tools.cxx:
        script.append("##export_var## CXX {}".format(_absolutize(ctx.workspace_name, tools.cxx)))

    copts = flags.cc
    cxxopts = flags.cxx
    if copts:
        script.append("##export_var## CFLAGS \"{} ${{CFLAGS:-}}\"".format(_join_flags_list(ctx.workspace_name, copts).replace("\"", "'")))
    if cxxopts:
        script.append("##export_var## CXXFLAGS \"{} ${{CXXFLAGS:-}}\"".format(_join_flags_list(ctx.workspace_name, cxxopts).replace("\"", "'")))

    if flags.cxx_linker_executable:
        script.append("##export_var## LDFLAGS \"{} ${{LDFLAGS:-}}\"".format(_join_flags_list(ctx.workspace_name, flags.cxx_linker_executable).replace("\"", "'")))

    script.append("##export_var## CMAKE {}".format(attrs.cmake_path))
    script.append("##export_var## NINJA {}".format(attrs.ninja_path))
    script.append("##export_var## PKG_CONFIG {}".format(attrs.pkg_config_path))

    root = detect_root(ctx.attr.lib_source)
    data = ctx.attr.data + ctx.attr.build_data

    if attrs.tool_prefix:
        tool_prefix = "{} ".format(
            expand_locations_and_make_variables(ctx, attrs.tool_prefix, "tool_prefix", data),
        )
    else:
        tool_prefix = ""

    meson_path = "{tool_prefix}{meson_path}".format(
        tool_prefix = tool_prefix,
        meson_path = attrs.meson_path,
    )

    target_args = dict(ctx.attr.target_args)

    # --- TODO: DEPRECATED, delete on a future release ------------------------
    # Convert the deprecated arguments into the new target_args argument. Fail
    # if there's a deprecated argument being used together with its new
    # target_args (e.g. setup_args and a "setup" target_args).

    deprecated = [
        ("setup", ctx.attr.setup_args),
        ("compile", ctx.attr.build_args),
        ("install", ctx.attr.install_args),
    ]

    for target_name, args_ in deprecated:
        if args_:
            if target_name in target_args:
                fail("Please migrate '{t}_args' to 'target_args[\"{t}\"]'".format(t = target_name))
            target_args[target_name] = args_

    # --- TODO: DEPRECATED, delete on a future release ------------------------

    # Expand target args
    for target_name, args_ in target_args.items():
        if target_name == "setup":
            args = expand_locations_and_make_variables(ctx, args_, "setup_args", data)
        else:
            args = [ctx.expand_location(arg, data) for arg in args_]

        target_args[target_name] = args

    script.append("{meson} setup --prefix={install_dir} {setup_args} {options} {source_dir}".format(
        meson = meson_path,
        install_dir = "$$INSTALLDIR$$",
        setup_args = " ".join(target_args.get("setup", [])),
        options = " ".join([
            "-D{}=\"{}\"".format(key, ctx.attr.options[key])
            for key in ctx.attr.options
        ]),
        source_dir = "$$EXT_BUILD_ROOT$$/" + root,
    ))

    targets = ctx.attr.targets

    # --- TODO: DEPRECATED, delete on a future release ------------------------
    targets = [
        t
        for t in targets
        if t != "install" or ctx.attr.install
    ]
    # --- TODO: DEPRECATED, delete on a future release ------------------------

    # NOTE:
    # introspect has an "old API" and doesn't work like other commands.
    # It requires a builddir argument and it doesn't have a flag to output to a
    # file, so it requires a redirect. And, most probably, it will remain like
    # this for the foreseable future (see
    # https://github.com/mesonbuild/meson/issues/8182#issuecomment-758183324).
    #
    # To keep things simple, we provide a basic API: users must supply the
    # introspection JSON file in `out_data_files`, and we offer a sensible
    # default for the introspect command that users can override if needed.
    if "introspect" in targets:
        if len(ctx.attr.out_data_files) != 1:
            msg = "Meson introspect expects a single JSON filename via "
            msg += "out_data_files; only one filename should be provided."
            fail(msg)

        introspect_file = ctx.attr.out_data_files[0]

        introspect_args = ["$$BUILD_TMPDIR$$"]
        introspect_args += target_args.get("introspect", ["--all", "--indent"])
        introspect_args += [">", "$$INSTALLDIR$$/{}".format(introspect_file)]

        target_args["introspect"] = introspect_args

    for target_name in targets:
        script.append("{meson} {target} {args}".format(
            meson = meson_path,
            target = target_name,
            args = " ".join(target_args.get(target_name, [])),
        ))

    return script

def _attrs():
    """Modifies the common set of attributes used by rules_foreign_cc and sets Meson specific attrs

    Returns:
        dict: Attributes of the `meson` rule
    """
    attrs = dict(CC_EXTERNAL_RULE_ATTRIBUTES)

    attrs.update({
        "build_args": attr.string_list(
            doc = "__deprecated__: please use `target_args` with `'build'` target key.",
            mandatory = False,
        ),
        "install": attr.bool(
            doc = "__deprecated__: please use `targets` if you want to skip install.",
            default = True,
        ),
        "install_args": attr.string_list(
            doc = "__deprecated__: please use `target_args` with `'install'` target key.",
            mandatory = False,
        ),
        "options": attr.string_dict(
            doc = "Meson `setup` options (converted to `-Dkey=value`)",
            mandatory = False,
            default = {},
        ),
        "setup_args": attr.string_list(
            doc = "__deprecated__: please use `target_args` with `'setup'` target key.",
            mandatory = False,
        ),
        "target_args": attr.string_list_dict(
            doc = "Dict of arguments for each of the Meson targets. The " +
                  "target name is the key and the list of args is the value.",
            mandatory = False,
        ),
        "targets": attr.string_list(
            doc = "A list of targets to run. Defaults to ['compile', 'install']",
            default = ["compile", "install"],
            mandatory = False,
        ),
    })
    return attrs

meson = rule(
    doc = (
        "Rule for building external libraries with [Meson](https://mesonbuild.com/)."
    ),
    attrs = _attrs(),
    fragments = CC_EXTERNAL_RULE_FRAGMENTS,
    output_to_genfiles = True,
    provides = [CcInfo],
    implementation = _meson_impl,
    toolchains = [
        "@rules_foreign_cc//toolchains:meson_toolchain",
        "@rules_foreign_cc//toolchains:cmake_toolchain",
        "@rules_foreign_cc//toolchains:ninja_toolchain",
        "@rules_foreign_cc//toolchains:pkgconfig_toolchain",
        "@rules_foreign_cc//foreign_cc/private/framework:shell_toolchain",
        "@bazel_tools//tools/cpp:toolchain_type",
    ],
)

def meson_with_requirements(name, requirements, **kwargs):
    """ Wrapper macro around Meson rule to add Python libraries required by the Meson build.

    Args:
        name: The target name
        requirements: List of Python "requirements", see https://github.com/bazelbuild/rules_python/tree/00545742ad2450863aeb82353d4275a1e5ed3f24#using-third_party-packages-as-dependencies
        **kwargs: Remaining keyword arguments
    """
    tags = kwargs.pop("tags", [])

    meson_tool(
        name = "meson_tool_for_{}".format(name),
        main = "@meson_src//:meson.py",
        data = ["@meson_src//:runtime"],
        requirements = requirements,
        tags = tags + ["manual"],
    )

    native_tool_toolchain(
        name = "built_meson_for_{}".format(name),
        env = {"MESON": "$(execpath :meson_tool_for_{})".format(name)},
        path = "$(execpath :meson_tool_for_{})".format(name),
        target = ":meson_tool_for_{}".format(name),
    )

    native.toolchain(
        name = "built_meson_toolchain_for_{}".format(name),
        toolchain = "built_meson_for_{}".format(name),
        toolchain_type = "@rules_foreign_cc//toolchains:meson_toolchain",
    )

    foreign_cc_rule_variant(
        name = name,
        rule = meson,
        toolchain = str(full_label("built_meson_toolchain_for_{}".format(name))),
        **kwargs
    )

def _absolutize(workspace_name, text, force = False):
    return absolutize_path_in_str(workspace_name, "$EXT_BUILD_ROOT/", text, force)

def _join_flags_list(workspace_name, flags):
    return " ".join([_absolutize(workspace_name, flag) for flag in flags])
