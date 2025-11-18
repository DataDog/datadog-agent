# buildifier: disable=module-docstring
load(":make_env_vars.bzl", "get_ldflags_make_vars", "get_make_env_vars")
load(":make_script.bzl", "pkgconfig_script")

# buildifier: disable=function-docstring
def create_configure_script(
        workspace_name,
        tools,
        flags,
        root,
        user_options,
        configure_prefix,
        configure_command,
        deps,
        inputs,
        env_vars,
        configure_in_place,
        prefix_flag,
        autoconf,
        autoconf_options,
        autoreconf,
        autoreconf_options,
        autogen,
        autogen_command,
        autogen_options,
        make_prefix,
        make_path,
        make_targets,
        make_args,
        executable_ldflags_vars,
        shared_ldflags_vars,
        is_msvc):
    ext_build_dirs = inputs.ext_build_dirs

    script = pkgconfig_script(ext_build_dirs)

    root_path = "$$EXT_BUILD_ROOT$$/{}".format(root)
    configure_path = "{}/{}".format(root_path, configure_command)
    if configure_in_place:
        script.append("##copy_dir_contents_to_dir## $$EXT_BUILD_ROOT$$/{} $$BUILD_TMPDIR$$".format(root))
        root_path = "$$BUILD_TMPDIR$$"
        configure_path = "{}/{}".format(root_path, configure_command)

    script.append("##export_var## MAKE {}".format(make_path))
    script.append("##enable_tracing##")

    if autogen:
        # NOCONFIGURE is pseudo standard and tells the script to not invoke configure.
        # We explicitly invoke configure later.
        autogen_env_vars = _get_autogen_env_vars(env_vars)
        script.append("{env_vars} \"{root_dir}/{autogen}\" {options}".format(
            env_vars = " ".join(["{}=\"{}\"".format(key, value) for (key, value) in autogen_env_vars.items()]),
            root_dir = root_path,
            autogen = autogen_command,
            options = " ".join(autogen_options),
        ).lstrip())

    env_vars_string = " ".join(["{}=\"{}\"".format(key, value) for (key, value) in env_vars.items()])

    if autoconf:
        script.append("{env_vars} {autoconf} {options}".format(
            env_vars = env_vars_string,
            # TODO: Pass autoconf via a toolchain
            autoconf = "autoconf",
            options = " ".join(autoconf_options),
        ).lstrip())

    if autoreconf:
        script.append("{env_vars} {autoreconf} {options}".format(
            env_vars = env_vars_string,
            # TODO: Pass autoreconf via a toolchain
            autoreconf = "autoreconf",
            options = " ".join(autoreconf_options),
        ).lstrip())

    script.append("##mkdirs## $$BUILD_TMPDIR$$/$$INSTALL_PREFIX$$")

    make_commands = []
    script.append("{env_vars} {prefix}\"{configure}\" {prefix_flag}$$BUILD_TMPDIR$$/$$INSTALL_PREFIX$$ {user_options}".format(
        env_vars = get_make_env_vars(workspace_name, tools, flags, env_vars, deps, inputs, is_msvc, make_commands),
        prefix = configure_prefix,
        configure = configure_path,
        prefix_flag = prefix_flag,
        user_options = " ".join(user_options),
    ))

    ldflags_make_vars = get_ldflags_make_vars(executable_ldflags_vars, shared_ldflags_vars, workspace_name, flags, env_vars, deps, inputs, is_msvc)

    make_commands = []
    for target in make_targets:
        make_commands.append("{prefix}{make} {make_vars} {target} {args}".format(
            prefix = make_prefix,
            make = make_path,
            make_vars = ldflags_make_vars,
            args = make_args,
            target = target,
        ))

    script.extend(make_commands)
    script.append("##disable_tracing##")
    script.append("##copy_dir_contents_to_dir## $$BUILD_TMPDIR$$/$$INSTALL_PREFIX$$ $$INSTALLDIR$$\n")

    return script

def _get_autogen_env_vars(autogen_env_vars):
    # Make a copy if necessary so we can set NOCONFIGURE.
    if autogen_env_vars.get("NOCONFIGURE"):
        return autogen_env_vars
    vars = {}
    for key in autogen_env_vars:
        vars[key] = autogen_env_vars.get(key)
    vars["NOCONFIGURE"] = "1"
    return vars
