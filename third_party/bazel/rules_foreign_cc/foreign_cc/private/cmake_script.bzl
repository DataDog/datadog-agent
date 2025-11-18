""" Contains all logic for calling CMake for building external libraries/binaries """

load("//foreign_cc/private:make_script.bzl", "pkgconfig_script")
load(":cc_toolchain_util.bzl", "absolutize_path_in_str")

def _escape_dquote_bash(text):
    """ Escape double quotes in flag lists for use in bash strings that set environment variables """

    # We use a starlark raw string to prevent the need to escape backslashes for starlark as well.
    return text.replace('"', r'\\\"')

def _escape_dquote_bash_crosstool(text):
    """ Escape double quotes in flag lists for use in bash strings to be passed to a HEREDOC.

        This requires an additional level of indirection as CMake requires the variables to be escaped inside the crosstool file
    """

    # We use a starlark raw string to prevent the need to escape backslashes for starlark as well.
    return text.replace('"', r'\\\\\\\"')

_TARGET_OS_PARAMS = {
    "android": {
        "ANDROID": "YES",
        "CMAKE_SYSTEM_NAME": "Linux",
    },
    "emscripten": {
        "CMAKE_SYSTEM_NAME": "Emscripten",
    },
    "linux": {
        "CMAKE_SYSTEM_NAME": "Linux",
    },
}

_TARGET_ARCH_PARAMS = {
    "aarch64": {
        "CMAKE_SYSTEM_PROCESSOR": "aarch64",
    },
    "ppc64le": {
        "CMAKE_SYSTEM_PROCESSOR": "ppc64le",
    },
    "s390x": {
        "CMAKE_SYSTEM_PROCESSOR": "s390x",
    },
    # Emscripten configures CMAKE_SYSTEM_PROCESSOR as follows for compatibility with libraries such as OpenCV
    # https://github.com/emscripten-core/emscripten/blob/79ee3d1/cmake/Modules/Platform/Emscripten.cmake#L23-L30
    "wasm32": {
        "CMAKE_SYSTEM_PROCESSOR": "x86",
    },
    "wasm64": {
        "CMAKE_SYSTEM_PROCESSOR": "x86_64",
    },
    "x86_64": {
        "CMAKE_SYSTEM_PROCESSOR": "x86_64",
    },
}

def create_cmake_script(
        workspace_name,
        current_label,
        target_os,
        target_arch,
        host_os,
        host_arch,
        generator,
        cmake_path,
        tools,
        flags,
        install_prefix,
        root,
        no_toolchain_file,
        user_cache,
        user_env,
        options,
        cmake_commands,
        include_dirs = [],
        cmake_prefix = None,
        is_debug_mode = True,
        ext_build_dirs = []):
    """Constructs CMake script to be passed to cc_external_rule_impl.

    Args:
        workspace_name: current workspace name
        current_label: The label of the target currently being built
        target_os: The target OS for the build
        target_arch: The target arch for the build
        host_os: The execution OS for the build
        host_arch: The execution arch for the build
        generator: The generator target for cmake to use
        cmake_path: The path to the cmake executable
        tools: cc_toolchain tools (CxxToolsInfo)
        flags: cc_toolchain flags (CxxFlagsInfo)
        install_prefix: value ot pass to CMAKE_INSTALL_PREFIX
        root: sources root relative to the $EXT_BUILD_ROOT
        no_toolchain_file: if False, CMake toolchain file will be generated, otherwise not
        user_cache: dictionary with user's values of cache initializers
        user_env: dictionary with user's values for CMake environment variables
        options: other CMake options specified by user
        cmake_commands: A list of cmake commands for building and installing targets
        include_dirs: Optional additional include directories. Defaults to [].
        cmake_prefix: Optional prefix before the cmake command (without the trailing space).
        is_debug_mode: If the compilation mode is `debug`. Defaults to True.
        ext_build_dirs: A list of gen_dirs for each foreign_cc dep.

    Returns:
        list: Lines of bash which make up the build script
    """

    merged_prefix_path = _merge_prefix_path(user_cache, include_dirs, ext_build_dirs)

    toolchain_dict = _fill_crossfile_from_toolchain(workspace_name, tools, flags, target_os)
    params = None

    # Avoid CMake passing the wrong linker flags when cross compiling
    # by setting CMAKE_SYSTEM_NAME and CMAKE_SYSTEM_PROCESSOR,
    # see https://github.com/bazel-contrib/rules_foreign_cc/issues/289,
    # and https://github.com/bazel-contrib/rules_foreign_cc/pull/1062
    # note: these parameters must be set in the toolchain file, not just in the
    # cache, and CMAKE_SYSTEM_PROCESSOR is ignored unless CMAKE_SYSTEM_NAME is
    # also set.
    if target_os == "unknown":
        # buildifier: disable=print
        print("target_os is unknown, please update foreign_cc/private/framework/platform.bzl and foreign_cc/private/cmake_script.bzl; triggered by", current_label)
    elif target_arch == "unknown":
        # buildifier: disable=print
        print("target_arch is unknown, please update foreign_cc/private/framework/platform.bzl and foreign_cc/private/cmake_script.bzl; triggered by", current_label)
    elif target_os != host_os or target_arch != host_arch:
        toolchain_dict.update(_TARGET_OS_PARAMS.get(target_os, {}))
        toolchain_dict.update(_TARGET_ARCH_PARAMS.get(target_arch, {}))

    keys_with_empty_values_in_user_cache = [key for key in user_cache if user_cache.get(key) == ""]

    if no_toolchain_file:
        params = _create_cache_entries_env_vars(toolchain_dict, user_cache, user_env)
    else:
        params = _create_crosstool_file_text(toolchain_dict, user_cache, user_env)

    build_type = params.cache.get(
        "CMAKE_BUILD_TYPE",
        "Debug" if is_debug_mode else "Release",
    )
    params.cache.update({
        "CMAKE_BUILD_TYPE": build_type,
        "CMAKE_INSTALL_PREFIX": install_prefix,
        "CMAKE_PREFIX_PATH": merged_prefix_path,
        "PKG_CONFIG_ARGN": "--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$",
    })

    # Give user the ability to suppress some value, taken from Bazel's toolchain,
    # or to suppress calculated CMAKE_BUILD_TYPE
    # If the user passes "CMAKE_BUILD_TYPE": "" (empty string),
    # CMAKE_BUILD_TYPE will not be passed to CMake
    _wipe_empty_values(params.cache, keys_with_empty_values_in_user_cache)

    # However, if no CMAKE_RANLIB was passed, pass the empty value for it explicitly,
    # as it is legacy and autodetection of ranlib made by CMake automatically
    # breaks some cross compilation builds,
    # see https://github.com/envoyproxy/envoy/pull/6991
    if not params.cache.get("CMAKE_RANLIB"):
        params.cache.update({"CMAKE_RANLIB": ""})

    set_env_vars = [
        "export {}=\"{}\"".format(key, _escape_dquote_bash(params.env[key]))
        for key in params.env
    ]
    str_cmake_cache_entries = " ".join([
        "-D{}=\"{}\"".format(key, _escape_dquote_bash_crosstool(params.cache[key]))
        for key in params.cache
    ])

    # Add definitions for all environment variables
    script = set_env_vars

    script += pkgconfig_script(ext_build_dirs)

    directory = "$$EXT_BUILD_ROOT$$/" + root

    script.append("##enable_tracing##")

    # Configure the CMake generate command
    cmake_prefixes = [cmake_prefix] if cmake_prefix else []
    script.append(" ".join(cmake_prefixes + [
        cmake_path,
        str_cmake_cache_entries,
        " ".join(options),
        # Generator is always set last and will override anything specified by the user
        "-G '{}'".format(generator),
        directory,
    ]))

    script.extend(cmake_commands)

    script.append("##disable_tracing##")

    return params.commands + script

def _wipe_empty_values(cache, keys_with_empty_values_in_user_cache):
    for key in keys_with_empty_values_in_user_cache:
        if cache.get(key) != None:
            cache.pop(key)

# From CMake documentation: ;-list of directories specifying installation prefixes to be searched...
def _merge_prefix_path(user_cache, include_dirs, ext_build_dirs):
    user_prefix = user_cache.get("CMAKE_PREFIX_PATH")
    values = ["$$EXT_BUILD_DEPS$$"] + include_dirs
    for ext_dir in ext_build_dirs:
        values.append("$$EXT_BUILD_DEPS$$/{}".format(ext_dir.basename))

    if user_prefix != None:
        # remove it, it is gonna be merged specifically
        user_cache.pop("CMAKE_PREFIX_PATH")
        values.append(user_prefix.strip("\"'"))
    return ";".join(values)

_CMAKE_ENV_VARS_FOR_CROSSTOOL = {
    "ASMFLAGS": struct(value = "CMAKE_ASM_FLAGS_INIT", replace = False),
    "CC": struct(value = "CMAKE_C_COMPILER", replace = True),
    "CFLAGS": struct(value = "CMAKE_C_FLAGS_INIT", replace = False),
    "CXX": struct(value = "CMAKE_CXX_COMPILER", replace = True),
    "CXXFLAGS": struct(value = "CMAKE_CXX_FLAGS_INIT", replace = False),
}

_CMAKE_CACHE_ENTRIES_CROSSTOOL = {
    "ANDROID": struct(value = "ANDROID", replace = False),
    "CMAKE_AR": struct(value = "CMAKE_AR", replace = True),
    "CMAKE_ASM_FLAGS": struct(value = "CMAKE_ASM_FLAGS_INIT", replace = False),
    "CMAKE_CXX_ARCHIVE_CREATE": struct(value = "CMAKE_CXX_ARCHIVE_CREATE", replace = False),
    "CMAKE_CXX_FLAGS": struct(value = "CMAKE_CXX_FLAGS_INIT", replace = False),
    "CMAKE_CXX_LINK_EXECUTABLE": struct(value = "CMAKE_CXX_LINK_EXECUTABLE", replace = True),
    "CMAKE_C_ARCHIVE_CREATE": struct(value = "CMAKE_C_ARCHIVE_CREATE", replace = False),
    "CMAKE_C_FLAGS": struct(value = "CMAKE_C_FLAGS_INIT", replace = False),
    "CMAKE_EXE_LINKER_FLAGS": struct(value = "CMAKE_EXE_LINKER_FLAGS_INIT", replace = False),
    "CMAKE_MODULE_LINKER_FLAGS": struct(value = "CMAKE_MODULE_LINKER_FLAGS_INIT", replace = False),
    "CMAKE_RANLIB": struct(value = "CMAKE_RANLIB", replace = True),
    "CMAKE_SHARED_LINKER_FLAGS": struct(value = "CMAKE_SHARED_LINKER_FLAGS_INIT", replace = False),
    "CMAKE_STATIC_LINKER_FLAGS": struct(value = "CMAKE_STATIC_LINKER_FLAGS_INIT", replace = False),
    "CMAKE_SYSTEM_NAME": struct(value = "CMAKE_SYSTEM_NAME", replace = False),
    "CMAKE_SYSTEM_PROCESSOR": struct(value = "CMAKE_SYSTEM_PROCESSOR", replace = False),
}

def _create_crosstool_file_text(toolchain_dict, user_cache, user_env):
    cache_entries = _dict_copy(user_cache)
    env_vars = _dict_copy(user_env)
    _move_dict_values(toolchain_dict, env_vars, _CMAKE_ENV_VARS_FOR_CROSSTOOL)
    _move_dict_values(toolchain_dict, cache_entries, _CMAKE_CACHE_ENTRIES_CROSSTOOL)

    lines = []
    crosstool_vars = []

    # The __var_* bash variables that are set here are a method to avoid
    # having to quote the values when they are expanded in the HEREDOC.
    # We could disable shell expansion by single quoting EOF in the HEREDOC
    # but then we loose the ability to expand other variables such as
    # $EXT_BUILD_DEPS and so we use this trick to leave expansion turned on in
    # the HEREDOC for the crosstool
    for key in toolchain_dict:
        crosstool_vars.append("__var_{}=\"{}\"".format(key, _escape_dquote_bash_crosstool(toolchain_dict[key])))
        if ("CMAKE_AR" == key):
            lines.append('set({} "$$__var_{}$$" {})'.format(
                key,
                key,
                'CACHE FILEPATH "Archiver"',
            ))
        else:
            lines.append('set({} "$$__var_{}$$")'.format(key, key))

    cache_entries.update({
        "CMAKE_TOOLCHAIN_FILE": "$$BUILD_TMPDIR$$/crosstool_bazel.cmake",
    })
    return struct(
        commands = sorted(crosstool_vars) + ["cat > crosstool_bazel.cmake << EOF"] + sorted(lines) + ["EOF", ""],
        env = env_vars,
        cache = cache_entries,
    )

def _dict_copy(d):
    out = {}
    if d:
        out.update(d)
    return out

def _create_cache_entries_env_vars(toolchain_dict, user_cache, user_env):
    _move_dict_values(toolchain_dict, user_env, _CMAKE_ENV_VARS_FOR_CROSSTOOL)
    _move_dict_values(toolchain_dict, user_cache, _CMAKE_CACHE_ENTRIES_CROSSTOOL)

    merged_env = _translate_from_toolchain_file(toolchain_dict, _CMAKE_ENV_VARS_FOR_CROSSTOOL)
    merged_cache = _translate_from_toolchain_file(toolchain_dict, _CMAKE_CACHE_ENTRIES_CROSSTOOL)

    # anything left in user's env_entries does not correspond to anything defined by toolchain
    # => simple merge
    merged_env.update(user_env)
    merged_cache.update(user_cache)

    return struct(
        commands = [],
        env = merged_env,
        cache = merged_cache,
    )

def _translate_from_toolchain_file(toolchain_dict, descriptor_map):
    reverse = _reverse_descriptor_dict(descriptor_map)
    cl_keyed_toolchain = dict()

    keys = toolchain_dict.keys()
    for key in keys:
        env_var_key = reverse.get(key)
        if env_var_key:
            cl_keyed_toolchain[env_var_key.value] = toolchain_dict.pop(key)
    return cl_keyed_toolchain

def _merge_toolchain_and_user_values(toolchain_dict, user_dict, descriptor_map):
    _move_dict_values(toolchain_dict, user_dict, descriptor_map)
    cl_keyed_toolchain = _translate_from_toolchain_file(toolchain_dict, descriptor_map)

    # anything left in user's env_entries does not correspond to anything defined by toolchain
    # => simple merge
    cl_keyed_toolchain.update(user_dict)
    return cl_keyed_toolchain

def _reverse_descriptor_dict(dict):
    out_dict = {}

    for key in dict:
        value = dict[key]
        out_dict[value.value] = struct(value = key, replace = value.replace)

    return out_dict

def _move_dict_values(target, source, descriptor_map):
    keys = source.keys()
    for key in keys:
        existing = descriptor_map.get(key)
        if existing:
            value = source.pop(key)
            if existing.replace or target.get(existing.value) == None:
                target[existing.value] = value
            else:
                target[existing.value] = target[existing.value] + " " + value

def _fill_crossfile_from_toolchain(workspace_name, tools, flags, target_os):
    dict = {}

    _sysroot = _find_in_cc_or_cxx(flags, "sysroot")
    if _sysroot:
        dict["CMAKE_SYSROOT"] = _absolutize(workspace_name, _sysroot)

    _ext_toolchain_cc = _find_flag_value(flags.cc, "gcc_toolchain")
    if _ext_toolchain_cc:
        dict["CMAKE_C_COMPILER_EXTERNAL_TOOLCHAIN"] = _absolutize(workspace_name, _ext_toolchain_cc)

    _ext_toolchain_cxx = _find_flag_value(flags.cxx, "gcc_toolchain")
    if _ext_toolchain_cxx:
        dict["CMAKE_CXX_COMPILER_EXTERNAL_TOOLCHAIN"] = _absolutize(workspace_name, _ext_toolchain_cxx)

    # Force convert tools paths to absolute using $EXT_BUILD_ROOT
    if tools.cc:
        dict["CMAKE_C_COMPILER"] = _absolutize(workspace_name, tools.cc, True)
    if tools.cxx:
        dict["CMAKE_CXX_COMPILER"] = _absolutize(workspace_name, tools.cxx, True)

    if tools.cxx_linker_static:
        dict["CMAKE_AR"] = _absolutize(workspace_name, tools.cxx_linker_static, True)
        if tools.cxx_linker_static.endswith("/libtool"):
            dict["CMAKE_C_ARCHIVE_CREATE"] = "<CMAKE_AR> %s <OBJECTS>" % \
                                             " ".join(flags.cxx_linker_static)
            dict["CMAKE_CXX_ARCHIVE_CREATE"] = "<CMAKE_AR> %s <OBJECTS>" % \
                                               " ".join(flags.cxx_linker_static)

    if tools.cxx_linker_executable and tools.cxx_linker_executable != tools.cxx:
        normalized_path = _absolutize(workspace_name, tools.cxx_linker_executable)
        dict["CMAKE_CXX_LINK_EXECUTABLE"] = " ".join([
            normalized_path,
            "<FLAGS>",
            "<CMAKE_CXX_LINK_FLAGS>",
            "<LINK_FLAGS>",
            "<OBJECTS>",
            "-o <TARGET>",
            "<LINK_LIBRARIES>",
        ])

    if flags.cc:
        dict["CMAKE_C_FLAGS_INIT"] = _join_flags_list(workspace_name, flags.cc)
    if flags.cxx:
        dict["CMAKE_CXX_FLAGS_INIT"] = _join_flags_list(workspace_name, flags.cxx)
    if flags.assemble:
        dict["CMAKE_ASM_FLAGS_INIT"] = _join_flags_list(workspace_name, flags.assemble)

    # todo this options are needed, but cause a bug because the flags are put in wrong order => keep this line
    #    if flags.cxx_linker_static:
    #        lines += [_set_list(ctx, "CMAKE_STATIC_LINKER_FLAGS_INIT", flags.cxx_linker_static)]
    if flags.cxx_linker_shared:
        dict["CMAKE_SHARED_LINKER_FLAGS_INIT"] = _join_flags_list(workspace_name, flags.cxx_linker_shared)

        # cxx_linker_shared will contain '-shared' or '-dynamiclib' on macos. This flag conflicts with "-bundle"
        # that is set by CMAKE based on platform. e.g.
        # https://gitlab.kitware.com/cmake/cmake/-/blob/master/Modules/Platform/Apple-Intel.cmake#L11
        # Therefore, for modules aka bundles we want to remove these flags.
        module_linker_flags = []
        if target_os == "macos":
            module_linker_flags = [flag for flag in flags.cxx_linker_shared if flag not in ["-shared", "-dynamiclib"]]
        else:
            module_linker_flags = flags.cxx_linker_shared
        dict["CMAKE_MODULE_LINKER_FLAGS_INIT"] = _join_flags_list(workspace_name, module_linker_flags)
    if flags.cxx_linker_executable:
        dict["CMAKE_EXE_LINKER_FLAGS_INIT"] = _join_flags_list(workspace_name, flags.cxx_linker_executable)

    # todo this is a kind of hacky way to handle this; I suspect once
    # https://github.com/bazelbuild/bazel/pull/23204 lands, it will be possible
    # to do this better.
    #
    # The problem being solved here is: if a toolchain wants to link the
    # toolchain libs statically, there are some flags that need to be passed.
    # Unfortunately, static linking is notoriously order-sensitive (if an
    # object needs a symbol, it can only be resolved by libraries _later_ than
    # it on the command line). This means there are scenarios where:
    # this works:
    #   gcc thing.o -o stuff -l:libstdc++.a
    # this fails with missing symbols (like std::cout):
    #   gcc -l:libstdc++.a -o stuff thing.o
    #
    # In other words, we need these flags to be in "<LINK_LIBRARIES>" and not
    # just "<LINK_FLAGS>", so they fall after the "<OBJECTS>" that might need
    # them and that is what this code does, by injecting these indicative flags
    # into CMAKE_CXX_STANDARD_LIBRARIES_INIT
    static_flags = []
    for flag in ("static-libstdc++", "static-libgcc", "l:libstdc++.a"):
        if flags.cxx_linker_shared and _find_flag_value(flags.cxx_linker_shared, flag):
            static_flags.append("-" + flag)
            continue

        if flags.cxx_linker_executable and _find_flag_value(flags.cxx_linker_executable, flag):
            static_flags.append("-" + flag)
            continue

    if static_flags:
        dict["CMAKE_CXX_STANDARD_LIBRARIES_INIT"] = _join_flags_list(workspace_name, static_flags)

    return dict

def _find_in_cc_or_cxx(flags, flag_name_no_dashes):
    _value = _find_flag_value(flags.cxx, flag_name_no_dashes)
    if _value:
        return _value
    return _find_flag_value(flags.cc, flag_name_no_dashes)

def _find_flag_value(list, flag_name_no_dashes):
    one_dash = "-" + flag_name_no_dashes.lstrip(" ")
    two_dash = "--" + flag_name_no_dashes.lstrip(" ")

    check_for_value = False
    for value in list:
        value = value.lstrip(" ")
        if check_for_value:
            return value.lstrip(" =")
        _tail = _tail_if_starts_with(value, one_dash)
        _tail = _tail_if_starts_with(value, two_dash) if _tail == None else _tail
        if _tail != None and len(_tail) > 0:
            return _tail.lstrip(" =")
        if _tail != None:
            check_for_value = True
    return None

def _tail_if_starts_with(str, start):
    if (str.startswith(start)):
        return str[len(start):]
    return None

def _absolutize(workspace_name, text, force = False):
    if text.strip(" ").startswith("C:") or text.strip(" ").startswith("c:"):
        return text

    # Use bash parameter substitution to replace backslashes with forward slashes as CMake fails if provided paths containing backslashes
    return absolutize_path_in_str(workspace_name, "$${EXT_BUILD_ROOT//\\\\//}$$/", text, force)

def _join_flags_list(workspace_name, flags):
    return " ".join([_absolutize(workspace_name, flag) for flag in flags])

export_for_test = struct(
    absolutize = _absolutize,
    tail_if_starts_with = _tail_if_starts_with,
    find_flag_value = _find_flag_value,
    fill_crossfile_from_toolchain = _fill_crossfile_from_toolchain,
    move_dict_values = _move_dict_values,
    reverse_descriptor_dict = _reverse_descriptor_dict,
    merge_toolchain_and_user_values = _merge_toolchain_and_user_values,
    CMAKE_ENV_VARS_FOR_CROSSTOOL = _CMAKE_ENV_VARS_FOR_CROSSTOOL,
    CMAKE_CACHE_ENTRIES_CROSSTOOL = _CMAKE_CACHE_ENTRIES_CROSSTOOL,
)
