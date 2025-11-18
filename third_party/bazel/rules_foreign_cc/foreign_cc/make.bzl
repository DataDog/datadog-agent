"""A rule for building projects using the [GNU Make](https://www.gnu.org/software/make/) build tool"""

load("@bazel_tools//tools/cpp:toolchain_utils.bzl", "find_cpp_toolchain")
load("@rules_cc//cc:defs.bzl", "CcInfo")
load(
    "//foreign_cc/private:cc_toolchain_util.bzl",
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
load("//foreign_cc/private:make_script.bzl", "create_make_script")
load("//foreign_cc/private:transitions.bzl", "foreign_cc_rule_variant")
load("//toolchains/native_tools:tool_access.bzl", "get_make_data")

def _make(ctx):
    make_data = get_make_data(ctx)

    tools_data = [make_data]

    attrs = create_attrs(
        ctx.attr,
        configure_name = "Make",
        create_configure_script = _create_make_script,
        tools_data = tools_data,
        make_path = make_data.path,
    )
    return cc_external_rule_impl(ctx, attrs)

def _create_make_script(configureParameters):
    ctx = configureParameters.ctx
    attrs = configureParameters.attrs
    inputs = configureParameters.inputs

    root = detect_root(ctx.attr.lib_source)

    tools = get_tools_info(ctx)
    flags = get_flags_info(ctx)

    data = ctx.attr.data + ctx.attr.build_data

    # Generate a list of arguments for make
    args = " ".join([
        ctx.expand_location(arg, data)
        for arg in ctx.attr.args
    ])

    user_env = expand_locations_and_make_variables(ctx, ctx.attr.env, "env", data)

    make_commands = []
    prefix = "{} ".format(expand_locations_and_make_variables(ctx, attrs.tool_prefix, "tool_prefix", data)) if attrs.tool_prefix else ""
    for target in ctx.attr.targets:
        make_commands.append("{prefix}{make} {target} {args} PREFIX={install_prefix}".format(
            prefix = prefix,
            make = attrs.make_path,
            args = args,
            target = target,
            install_prefix = ctx.attr.install_prefix,
        ))

    cc_toolchain = find_cpp_toolchain(ctx)
    is_msvc = cc_toolchain.compiler == "msvc-cl"

    return create_make_script(
        workspace_name = ctx.workspace_name,
        tools = tools,
        flags = flags,
        root = root,
        deps = ctx.attr.deps,
        inputs = inputs,
        env_vars = user_env,
        make_prefix = prefix,
        make_path = attrs.make_path,
        make_targets = ctx.attr.targets,
        make_args = args,
        make_install_prefix = ctx.attr.install_prefix,
        executable_ldflags_vars = ctx.attr.executable_ldflags_vars,
        shared_ldflags_vars = ctx.attr.shared_ldflags_vars,
        is_msvc = is_msvc,
    )

def _attrs():
    attrs = dict(CC_EXTERNAL_RULE_ATTRIBUTES)
    attrs.update({
        "args": attr.string_list(
            doc = "A list of arguments to pass to the call to `make`",
        ),
        "executable_ldflags_vars": attr.string_list(
            doc = (
                "A string list of variable names use as LDFLAGS for executables. These variables " +
                "will be passed to the make command as make vars and overwrite what is defined in " +
                "the Makefile."
            ),
            mandatory = False,
            default = [],
        ),
        "install_prefix": attr.string(
            doc = (
                "Install prefix, i.e. relative path to where to install the result of the build. " +
                "Passed as an arg to \"make\" as PREFIX=<install_prefix>."
            ),
            mandatory = False,
            default = "$$INSTALLDIR$$",
        ),
        "shared_ldflags_vars": attr.string_list(
            doc = (
                "A string list of variable names use as LDFLAGS for shared libraries. These variables " +
                "will be passed to the make command as make vars and overwrite what is defined in " +
                "the Makefile."
            ),
            mandatory = False,
            default = [],
        ),
        "targets": attr.string_list(
            doc = (
                "A list of targets within the foreign build system to produce. An empty string (`\"\"`) will result in " +
                "a call to the underlying build system with no explicit target set. However, in order to extract build " +
                "outputs, you must execute at least an equivalent of make install, and have your make file copy the build " +
                "outputs into the directory specified by `install_prefix`."
            ),
            mandatory = False,
            default = ["", "install"],
        ),
    })
    return attrs

make = rule(
    doc = (
        "Rule for building external libraries with GNU Make. " +
        "GNU Make commands (make and make install by default) are invoked with PREFIX=\"install\" " +
        "(by default), and other environment variables for compilation and linking, taken from Bazel C/C++ " +
        "toolchain and passed dependencies. " +
        "Not all Makefiles will work equally well here, and some may require patching." +
        "Your Makefile must either support passing the install prefix using the PREFIX flag, or " +
        "it needs to have a different way to pass install prefix to it. An equivalent of " +
        "make install MUST be specified as one of the targets." +
        "This is because all the paths with param names prefixed by out_* are expressed " +
        "as relative to INSTALLDIR, not the source directory." +
        "That is, if you execute only make, but not make install, this rule will not be able " +
        "to pick up any build outputs. Finally, your make install rule must dereference symlinks " +
        "to ensure that the installed files don't end up being symlinks to files in the sandbox. " +
        "For example, installation lines like `cp $SOURCE $DEST` must become `cp -L $SOURCE $DEST`, " +
        "as the -L will ensure that symlinks are dereferenced."
    ),
    attrs = _attrs(),
    fragments = CC_EXTERNAL_RULE_FRAGMENTS,
    output_to_genfiles = True,
    provides = [CcInfo],
    implementation = _make,
    toolchains = [
        "@rules_foreign_cc//toolchains:make_toolchain",
        "@rules_foreign_cc//foreign_cc/private/framework:shell_toolchain",
        "@bazel_tools//tools/cpp:toolchain_type",
    ],
)

def make_variant(name, toolchain, **kwargs):
    """ Wrapper macro around the make() rule to force usage of the given make variant toolchain.

    Args:
        name: The target name
        toolchain: The desired make variant toolchain to use, e.g. @rules_foreign_cc//toolchains:preinstalled_nmake_toolchain
        **kwargs: Remaining keyword arguments
    """
    foreign_cc_rule_variant(
        name = name,
        rule = make,
        toolchain = toolchain,
        **kwargs
    )
