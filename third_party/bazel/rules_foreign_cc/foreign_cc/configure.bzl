"""A rule for building projects using the [Configure+Make](https://www.gnu.org/prep/standards/html_node/Configuration.html)
build tool
"""

load("@bazel_tools//tools/cpp:toolchain_utils.bzl", "find_cpp_toolchain")
load("@rules_cc//cc:defs.bzl", "CcInfo")
load(
    "//foreign_cc/private:cc_toolchain_util.bzl",
    "get_flags_info",
    "get_tools_info",
)
load("//foreign_cc/private:configure_script.bzl", "create_configure_script")
load("//foreign_cc/private:detect_root.bzl", "detect_root")
load("//foreign_cc/private:detect_xcompile.bzl", "detect_xcompile")
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
    "//toolchains/native_tools:tool_access.bzl",
    "get_autoconf_data",
    "get_automake_data",
    "get_m4_data",
    "get_make_data",
    "get_pkgconfig_data",
)

def _configure_make(ctx):
    make_data = get_make_data(ctx)
    pkg_config_data = get_pkgconfig_data(ctx)
    autoconf_data = get_autoconf_data(ctx)
    automake_data = get_automake_data(ctx)
    m4_data = get_m4_data(ctx)

    tools_data = [
        make_data,
        pkg_config_data,
        autoconf_data,
        automake_data,
        m4_data,
    ]

    if ctx.attr.autogen and not ctx.attr.configure_in_place:
        fail("`autogen` requires `configure_in_place = True`. Please update {}".format(
            ctx.label,
        ))

    if ctx.attr.autoconf and not ctx.attr.configure_in_place:
        fail("`autoconf` requires `configure_in_place = True`. Please update {}".format(
            ctx.label,
        ))

    if ctx.attr.autoreconf and not ctx.attr.configure_in_place:
        fail("`autoreconf` requires `configure_in_place = True`. Please update {}".format(
            ctx.label,
        ))

    attrs = create_attrs(
        ctx.attr,
        configure_name = "Configure",
        create_configure_script = _create_configure_script,
        tools_data = tools_data,
        make_path = make_data.path,
    )
    return cc_external_rule_impl(ctx, attrs)

def _create_configure_script(configureParameters):
    ctx = configureParameters.ctx
    attrs = configureParameters.attrs
    inputs = configureParameters.inputs

    tools = get_tools_info(ctx)
    flags = get_flags_info(ctx)

    define_install_prefix = ["export INSTALL_PREFIX=\"" + _get_install_prefix(ctx) + "\""]

    data = ctx.attr.data + ctx.attr.build_data

    # Generate a list of arguments for make
    args = " ".join([
        ctx.expand_location(arg, data)
        for arg in ctx.attr.args
    ])

    user_env = expand_locations_and_make_variables(ctx, ctx.attr.env, "env", data)

    prefix = "{} ".format(expand_locations_and_make_variables(ctx, attrs.tool_prefix, "tool_prefix", data)) if attrs.tool_prefix else ""
    configure_prefix = "{} ".format(expand_locations_and_make_variables(ctx, ctx.attr.configure_prefix, "configure_prefix", data)) if ctx.attr.configure_prefix else ""
    configure_options = [expand_locations_and_make_variables(ctx, option, "configure_option", data) for option in ctx.attr.configure_options] if ctx.attr.configure_options else []

    xcompile_options = detect_xcompile(ctx)
    if xcompile_options:
        configure_options.extend(xcompile_options)

    cc_toolchain = find_cpp_toolchain(ctx)
    is_msvc = cc_toolchain.compiler == "msvc-cl"

    configure = create_configure_script(
        workspace_name = ctx.workspace_name,
        tools = tools,
        flags = flags,
        root = detect_root(ctx.attr.lib_source),
        user_options = configure_options,
        configure_prefix = configure_prefix,
        configure_command = ctx.attr.configure_command,
        deps = ctx.attr.deps,
        inputs = inputs,
        env_vars = user_env,
        configure_in_place = ctx.attr.configure_in_place,
        prefix_flag = ctx.attr.prefix_flag,
        autoconf = ctx.attr.autoconf,
        autoconf_options = ctx.attr.autoconf_options,
        autoreconf = ctx.attr.autoreconf,
        autoreconf_options = ctx.attr.autoreconf_options,
        autogen = ctx.attr.autogen,
        autogen_command = ctx.attr.autogen_command,
        autogen_options = ctx.attr.autogen_options,
        make_prefix = prefix,
        make_path = attrs.make_path,
        make_targets = ctx.attr.targets,
        make_args = args,
        executable_ldflags_vars = ctx.attr.executable_ldflags_vars,
        shared_ldflags_vars = ctx.attr.shared_ldflags_vars,
        is_msvc = is_msvc,
    )
    return define_install_prefix + configure

def _get_install_prefix(ctx):
    if ctx.attr.install_prefix:
        return ctx.attr.install_prefix
    if ctx.attr.lib_name:
        return ctx.attr.lib_name
    return ctx.attr.name

def _attrs():
    attrs = dict(CC_EXTERNAL_RULE_ATTRIBUTES)
    attrs.update({
        "args": attr.string_list(
            doc = "A list of arguments to pass to the call to `make`",
        ),
        "autoconf": attr.bool(
            mandatory = False,
            default = False,
            doc = (
                "Set to True if 'autoconf' should be invoked before 'configure', " +
                "currently requires `configure_in_place` to be True."
            ),
        ),
        "autoconf_options": attr.string_list(
            doc = "Any options to be put in the 'autoconf.sh' command line.",
        ),
        "autogen": attr.bool(
            doc = (
                "Set to True if 'autogen.sh' should be invoked before 'configure', " +
                "currently requires `configure_in_place` to be True."
            ),
            mandatory = False,
            default = False,
        ),
        "autogen_command": attr.string(
            doc = (
                "The name of the autogen script file, default: autogen.sh. " +
                "Many projects use autogen.sh however the Autotools FAQ recommends bootstrap " +
                "so we provide this option to support that."
            ),
            default = "autogen.sh",
        ),
        "autogen_options": attr.string_list(
            doc = "Any options to be put in the 'autogen.sh' command line.",
        ),
        "autoreconf": attr.bool(
            doc = (
                "Set to True if 'autoreconf' should be invoked before 'configure.', " +
                "currently requires `configure_in_place` to be True."
            ),
            mandatory = False,
            default = False,
        ),
        "autoreconf_options": attr.string_list(
            doc = "Any options to be put in the 'autoreconf.sh' command line.",
        ),
        "configure_command": attr.string(
            doc = (
                "The name of the configuration script file, default: configure. " +
                "The file must be in the root of the source directory."
            ),
            default = "configure",
        ),
        "configure_in_place": attr.bool(
            doc = (
                "Set to True if 'configure' should be invoked in place, i.e. from its enclosing " +
                "directory."
            ),
            mandatory = False,
            default = False,
        ),
        "configure_options": attr.string_list(
            doc = "Any options to be put on the 'configure' command line.",
        ),
        "configure_prefix": attr.string(
            doc = "A prefix for the call to the `configure_command`.",
        ),
        "configure_xcompile": attr.bool(
            doc = (
                "If this is set and an xcompile scenario is detected, pass the necessary autotools flags."
            ),
            default = False,
        ),
        "executable_ldflags_vars": attr.string_list(
            doc = (
                "A list of variable names use as LDFLAGS for executables. These variables " +
                "will be passed to the make command as make vars and overwrite what is defined in " +
                "the Makefile."
            ),
            mandatory = False,
            default = [],
        ),
        "install_prefix": attr.string(
            doc = (
                "Install prefix, i.e. relative path to where to install the result of the build. " +
                "Passed to the 'configure' script with the flag specified by prefix_flag."
            ),
            mandatory = False,
        ),
        "prefix_flag": attr.string(
            doc = (
                "The flag to specify the install directory prefix with."
            ),
            mandatory = False,
            default = "--prefix=",
        ),
        "shared_ldflags_vars": attr.string_list(
            doc = (
                "A list of variable names use as LDFLAGS for shared libraries. These variables " +
                "will be passed to the make command as make vars and overwrite what is defined in " +
                "the Makefile."
            ),
            mandatory = False,
            default = [],
        ),
        "targets": attr.string_list(
            doc = (
                "A list of targets within the foreign build system to produce. An empty string (`\"\"`) will result in " +
                "a call to the underlying build system with no explicit target set"
            ),
            mandatory = False,
            default = ["", "install"],
        ),
    })

    return attrs

configure_make = rule(
    doc = (
        "Rule for building external libraries with configure-make pattern. " +
        "Some 'configure' script is invoked with --prefix=install (by default), " +
        "and other parameters for compilation and linking, taken from Bazel C/C++ " +
        "toolchain and passed dependencies. " +
        "After configuration, GNU Make is called."
    ),
    attrs = _attrs(),
    fragments = CC_EXTERNAL_RULE_FRAGMENTS,
    output_to_genfiles = True,
    provides = [CcInfo],
    implementation = _configure_make,
    toolchains = [
        "@rules_foreign_cc//toolchains:autoconf_toolchain",
        "@rules_foreign_cc//toolchains:automake_toolchain",
        "@rules_foreign_cc//toolchains:make_toolchain",
        "@rules_foreign_cc//toolchains:m4_toolchain",
        "@rules_foreign_cc//toolchains:pkgconfig_toolchain",
        "@rules_foreign_cc//foreign_cc/private/framework:shell_toolchain",
        "@bazel_tools//tools/cpp:toolchain_type",
    ],
)

def configure_make_variant(name, toolchain, **kwargs):
    """ Wrapper macro around the configure_make() rule to force usage of the given make variant toolchain.

    Args:
        name: The target name
        toolchain: The desired make variant toolchain to use, e.g. @rules_foreign_cc//toolchains:preinstalled_nmake_toolchain
        **kwargs: Remaining keyword arguments
    """
    foreign_cc_rule_variant(
        name = name,
        rule = configure_make,
        toolchain = toolchain,
        **kwargs
    )
