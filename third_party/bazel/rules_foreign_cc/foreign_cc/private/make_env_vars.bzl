"""Helper methods to assemble make env variables from Bazel information."""

load(":cc_toolchain_util.bzl", "absolutize_path_in_str")
load(":framework.bzl", "get_foreign_cc_dep")

# buildifier: disable=function-docstring
def get_make_env_vars(
        workspace_name,
        tools,
        flags,
        user_vars,
        deps,
        inputs,
        is_msvc,
        make_commands):
    vars = _get_make_variables(workspace_name, tools, flags, user_vars, make_commands)
    deps_flags = _define_deps_flags(deps, inputs, is_msvc)

    # For cross-compilation.
    if "RANLIB" not in vars.keys():
        vars["RANLIB"] = [":"]

    if "LDFLAGS" in vars.keys():
        vars["LDFLAGS"] = vars["LDFLAGS"] + deps_flags.libs
    else:
        vars["LDFLAGS"] = deps_flags.libs

    # -I flags should be put into preprocessor flags, CPPFLAGS
    # https://www.gnu.org/software/autoconf/manual/autoconf-2.63/html_node/Preset-Output-Variables.html
    if "CPPFLAGS" in vars.keys():
        vars["CPPFLAGS"] = vars["CPPFLAGS"] + deps_flags.flags
    else:
        vars["CPPFLAGS"] = deps_flags.flags

    return " ".join(["{}=\"{}\""
        .format(key, _join_flags_list(workspace_name, vars[key])) for key in vars])

# buildifier: disable=function-docstring
def get_ldflags_make_vars(
        executable_ldflags_vars,
        shared_ldflags_vars,
        workspace_name,
        flags,
        user_vars,
        deps,
        inputs,
        is_msvc):
    vars = _get_ldflags_vars(executable_ldflags_vars, shared_ldflags_vars, flags, user_vars)

    deps_flags = _define_deps_flags(deps, inputs, is_msvc)
    for key in vars.keys():
        vars[key] = vars[key] + deps_flags.libs

    return " ".join(["{}=\"{}\""
        .format(key, _join_flags_list(workspace_name, vars[key])) for key in vars])

def _define_deps_flags(deps, inputs, is_msvc):
    # It is very important to keep the order for the linker => put them into list
    lib_dirs = []

    # msvc compiler uses -LIBPATH instead of -L
    lib_flag = "-LIBPATH:" if is_msvc else "-L"

    # Here go libraries built with Bazel
    gen_dirs_set = {}
    for lib in inputs.libs:
        dir_ = lib.dirname
        if not gen_dirs_set.get(dir_):
            gen_dirs_set[dir_] = 1
            lib_dirs.append(lib_flag + "$$EXT_BUILD_ROOT$$/" + dir_)

    include_dirs_set = {}
    for include_dir in inputs.include_dirs:
        include_dirs_set[include_dir] = "-I$$EXT_BUILD_ROOT$$/" + include_dir
    for header in inputs.headers:
        include_dir = header.dirname
        if not include_dirs_set.get(include_dir):
            include_dirs_set[include_dir] = "-I$$EXT_BUILD_ROOT$$/" + include_dir
    include_dirs = include_dirs_set.values()

    # For the external libraries, we need to refer to the places where
    # we copied the dependencies ($EXT_BUILD_DEPS/<lib_name>), because
    # we also want configure to find that same files with pkg-config
    # -config or other mechanics.
    # Since we need the names of include and lib directories under
    # the $EXT_BUILD_DEPS/<lib_name>, we ask the provider.
    gen_dirs_set = {}
    for dep in deps:
        external_deps = get_foreign_cc_dep(dep)
        if external_deps:
            for artifact in external_deps.artifacts.to_list():
                if not gen_dirs_set.get(artifact.gen_dir):
                    gen_dirs_set[artifact.gen_dir] = 1

                    dir_name = artifact.gen_dir.basename
                    include_dirs.append("-I$$EXT_BUILD_DEPS$$/{}/{}".format(dir_name, artifact.include_dir_name))
                    lib_dirs.append(lib_flag + "$$EXT_BUILD_DEPS$$/{}/{}".format(dir_name, artifact.lib_dir_name))

    return struct(
        libs = lib_dirs,
        flags = include_dirs,
    )

# See https://www.gnu.org/software/make/manual/html_node/Implicit-Variables.html
_MAKE_FLAGS = {
    "ARFLAGS": "cxx_linker_static",
    # AR_FLAGS is sometimes used
    "AR_FLAGS": "cxx_linker_static",
    "ASFLAGS": "assemble",
    "CFLAGS": "cc",
    "CXXFLAGS": "cxx",
    "LDFLAGS": "cxx_linker_executable",
    # missing: cxx_linker_shared
}

_MAKE_TOOLS = {
    "AR": "cxx_linker_static",
    "CC": "cc",
    "CXX": "cxx",
    "LD": "ld",
    # missing: cxx_linker_executable
}

def _get_make_variables(workspace_name, tools, flags, user_env_vars, make_commands):
    vars = _merge_env_vars(flags, _MAKE_FLAGS, user_env_vars)

    # Add user defined CPPFLAGS
    user_cpp_flags = [flag for flag in user_env_vars.get("CPPFLAGS", "").split(" ") if flag]
    if user_cpp_flags:
        vars["CPPFLAGS"] = user_cpp_flags

    tools_dict = {}
    for tool in _MAKE_TOOLS:
        tool_value = getattr(tools, _MAKE_TOOLS[tool])
        if tool_value:
            # Force absolutize of tool paths, which may relative to the exec root (e.g. hermetic toolchains built from source)
            tool_value_absolute = _absolutize(workspace_name, tool_value, True)

            # There are 2 conditions where we need to wrap the tool path in double quotes:
            # 1. If the tool path contains whitespaces (e.g. C:\Program Files\...),
            #    MSYS2 requires that the path is wrapped in double quotes.
            # 2. If nmake is used, it requires the tool path to be wrapped in double quotes,
            #    otherwise nmake will output the command as a string instead of executing it.
            if " " in tool_value_absolute or _nmake_in_make_commands(make_commands):
                tool_value_absolute = "\\\"" + tool_value_absolute + "\\\""

            tools_dict[tool] = [tool_value_absolute]

    # Replace tools paths if user passed other values
    for user_var in user_env_vars:
        toolchain_val = tools_dict.get(user_var)
        if toolchain_val:
            tools_dict[user_var] = [user_env_vars[user_var]]

    vars.update(tools_dict)

    # Do not put in the other user-defined env variables at this point as they
    # have already been exported globally by the prelude.

    return vars

def _get_ldflags_vars(
        executable_ldflags_vars,
        shared_ldflags_vars,
        flags,
        user_env_vars):
    executable_ldflags = {var: "cxx_linker_executable" for var in executable_ldflags_vars}
    shared_ldflags = {var: "cxx_linker_shared" for var in shared_ldflags_vars}

    linker_make_flags = {}
    for ldflags in [executable_ldflags, shared_ldflags]:
        linker_make_flags.update(ldflags)

    return _merge_env_vars(flags, linker_make_flags, user_env_vars)

def _merge_env_vars(flags, make_flags, user_env_vars):
    vars = {}

    for flag in make_flags:
        toolchain_flags = getattr(flags, make_flags[flag], [])
        user_flags = [
            user_flag
            for user_flag in user_env_vars.get(flag, "").split(" ")
            if user_flag
        ]
        if toolchain_flags or user_flags:
            vars[flag] = toolchain_flags + user_flags
    return vars

def _absolutize(workspace_name, text, force = False):
    return absolutize_path_in_str(workspace_name, "$$EXT_BUILD_ROOT$$/", text, force)

def _join_flags_list(workspace_name, flags):
    return " ".join([_absolutize(workspace_name, flag) for flag in flags])

def _nmake_in_make_commands(make_commands):
    return make_commands and "nmake.exe" in make_commands[0]
