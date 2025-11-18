""" Unit tests for CMake script creation """

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")

# buildifier: disable=bzl-visibility
load("//foreign_cc/private:cc_toolchain_util.bzl", "CxxFlagsInfo", "CxxToolsInfo")

# buildifier: disable=bzl-visibility
load("//foreign_cc/private:cmake_script.bzl", "create_cmake_script", "export_for_test")

def _absolutize_test(ctx):
    env = unittest.begin(ctx)

    cases = {
        "-Lexternal/cmake/aaa": "-L$${EXT_BUILD_ROOT//\\\\//}$$/external/cmake/aaa",
        "/abs/a12": "/abs/a12",
        "abs/a12": "abs/a12",
        "external/cmake/aaa": "$${EXT_BUILD_ROOT//\\\\//}$$/external/cmake/aaa",
        "name=ws/cmake/aaa": "name=$${EXT_BUILD_ROOT//\\\\//}$$/ws/cmake/aaa",
        "ws/cmake/aaa": "$${EXT_BUILD_ROOT//\\\\//}$$/ws/cmake/aaa",
    }

    for case in cases:
        res = export_for_test.absolutize("ws", case)
        asserts.equals(env, cases[case], res)

    return unittest.end(env)

def _tail_extraction_test(ctx):
    env = unittest.begin(ctx)

    res = export_for_test.tail_if_starts_with("absolutely", "abs")
    asserts.equals(env, "olutely", res)

    res = export_for_test.tail_if_starts_with("--option=value", "-option")
    asserts.equals(env, None, res)

    res = export_for_test.tail_if_starts_with("--option=value", "--option")
    asserts.equals(env, "=value", res)

    return unittest.end(env)

def _find_flag_value_test(ctx):
    env = unittest.begin(ctx)

    found_cases = [
        ["--gcc_toolchain=/abc/def"],
        ["--gcc_toolchain =/abc/def"],
        ["--gcc_toolchain= /abc/def"],
        ["--gcc_toolchain = /abc/def"],
        ["  --gcc_toolchain = /abc/def"],
        ["--gcc_toolchain", "=/abc/def"],
        ["--gcc_toolchain", "/abc/def"],
        ["-gcc_toolchain", "/abc/def"],
        ["-gcc_toolchain=/abc/def"],
        ["-gcc_toolchain = /abc/def"],
        ["--gcc_toolchain /abc/def"],
    ]

    for case in found_cases:
        res = export_for_test.find_flag_value(case, "gcc_toolchain")
        asserts.equals(env, "/abc/def", res, msg = "Not equals: " + str(case))

    not_found_cases = [
        ["--gcc_toolchainn=/abc/def"],
        ["--gcc_toolchain abc/def"],
    ]
    for case in not_found_cases:
        res = export_for_test.find_flag_value(case, "gcc_toolchain")
        asserts.false(env, "/abc/def" == res, msg = "Equals: " + str(case))

    return unittest.end(env)

def _fill_crossfile_from_toolchain_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "/some-cc-value",
        cxx = "external/cxx-value",
        cxx_linker_static = "/cxx_linker_static",
        cxx_linker_executable = "ws/cxx_linker_executable",
    )

    cases = {
        # format: target_os: (input_flags, expected_flags)
        "macos": (["-shared", "-dynamiclib", "-bundle"], ["-bundle"]),
        "unknown": (["shared1", "shared2"], ["shared1", "shared2"]),
    }

    for target_os, inputs in cases.items():
        flags = CxxFlagsInfo(
            cc = ["-cc-flag", "-gcc_toolchain", "cc-toolchain"],
            cxx = ["--quoted=\"abc def\"", "--sysroot=/abc/sysroot", "--gcc_toolchain", "cxx-toolchain"],
            cxx_linker_shared = inputs[0],
            cxx_linker_static = ["static"],
            cxx_linker_executable = ["executable"],
            assemble = ["assemble"],
        )

        res = export_for_test.fill_crossfile_from_toolchain("ws", tools, flags, target_os)

        expected = {
            "CMAKE_AR": "/cxx_linker_static",
            "CMAKE_ASM_FLAGS_INIT": "assemble",
            "CMAKE_CXX_COMPILER": "$${EXT_BUILD_ROOT//\\\\//}$$/external/cxx-value",
            "CMAKE_CXX_COMPILER_EXTERNAL_TOOLCHAIN": "cxx-toolchain",
            # Quoted args are escaped when crossfile is written to a script in create_cmake_script
            "CMAKE_CXX_FLAGS_INIT": "--quoted=\"abc def\" --sysroot=/abc/sysroot --gcc_toolchain cxx-toolchain",
            "CMAKE_CXX_LINK_EXECUTABLE": "$${EXT_BUILD_ROOT//\\\\//}$$/ws/cxx_linker_executable <FLAGS> <CMAKE_CXX_LINK_FLAGS> <LINK_FLAGS> <OBJECTS> -o <TARGET> <LINK_LIBRARIES>",
            "CMAKE_C_COMPILER": "/some-cc-value",
            "CMAKE_C_COMPILER_EXTERNAL_TOOLCHAIN": "cc-toolchain",
            "CMAKE_C_FLAGS_INIT": "-cc-flag -gcc_toolchain cc-toolchain",
            "CMAKE_EXE_LINKER_FLAGS_INIT": "executable",
            "CMAKE_MODULE_LINKER_FLAGS_INIT": " ".join(inputs[1]),
            "CMAKE_SHARED_LINKER_FLAGS_INIT": " ".join(inputs[0]),
            "CMAKE_SYSROOT": "/abc/sysroot",
        }

        for key in expected:
            asserts.equals(env, expected[key], res[key])

    return unittest.end(env)

def _move_dict_values_test(ctx):
    env = unittest.begin(ctx)

    target = {
        "CMAKE_CXX_COMPILER": "$EXT_BUILD_ROOT/external/cxx-value",
        "CMAKE_CXX_LINK_EXECUTABLE": "was",
        "CMAKE_C_COMPILER": "some-cc-value",
        "CMAKE_C_FLAGS_INIT": "-cc-flag -gcc_toolchain cc-toolchain",
    }
    source_env = {
        "CC": "sink-cc-value",
        "CFLAGS": "--from-env",
        "CUSTOM": "YES",
        "CXX": "sink-cxx-value",
    }
    source_cache = {
        "CMAKE_ASM_FLAGS": "assemble",
        "CMAKE_CXX_LINK_EXECUTABLE": "became",
        "CMAKE_C_FLAGS": "--additional-flag",
        "CUSTOM": "YES",
    }
    export_for_test.move_dict_values(
        target,
        source_env,
        export_for_test.CMAKE_ENV_VARS_FOR_CROSSTOOL,
    )
    export_for_test.move_dict_values(
        target,
        source_cache,
        export_for_test.CMAKE_CACHE_ENTRIES_CROSSTOOL,
    )

    expected_target = {
        "CMAKE_ASM_FLAGS_INIT": "assemble",
        "CMAKE_CXX_COMPILER": "sink-cxx-value",
        "CMAKE_CXX_LINK_EXECUTABLE": "became",
        "CMAKE_C_COMPILER": "sink-cc-value",
        "CMAKE_C_FLAGS_INIT": "-cc-flag -gcc_toolchain cc-toolchain --from-env --additional-flag",
    }
    for key in expected_target:
        asserts.equals(env, expected_target[key], target[key])

    asserts.equals(env, "YES", source_env["CUSTOM"])
    asserts.equals(env, "YES", source_cache["CUSTOM"])
    asserts.equals(env, 1, len(source_env))
    asserts.equals(env, 1, len(source_cache))

    return unittest.end(env)

def _reverse_descriptor_dict_test(ctx):
    env = unittest.begin(ctx)

    res = export_for_test.reverse_descriptor_dict(export_for_test.CMAKE_CACHE_ENTRIES_CROSSTOOL)
    expected = {
        "CMAKE_AR": struct(value = "CMAKE_AR", replace = True),
        "CMAKE_ASM_FLAGS_INIT": struct(value = "CMAKE_ASM_FLAGS", replace = False),
        "CMAKE_CXX_FLAGS_INIT": struct(value = "CMAKE_CXX_FLAGS", replace = False),
        "CMAKE_CXX_LINK_EXECUTABLE": struct(value = "CMAKE_CXX_LINK_EXECUTABLE", replace = True),
        "CMAKE_C_FLAGS_INIT": struct(value = "CMAKE_C_FLAGS", replace = False),
        "CMAKE_EXE_LINKER_FLAGS_INIT": struct(value = "CMAKE_EXE_LINKER_FLAGS", replace = False),
        "CMAKE_MODULE_LINKER_FLAGS_INIT": struct(value = "CMAKE_MODULE_LINKER_FLAGS", replace = False),
        "CMAKE_SHARED_LINKER_FLAGS_INIT": struct(value = "CMAKE_SHARED_LINKER_FLAGS", replace = False),
        "CMAKE_STATIC_LINKER_FLAGS_INIT": struct(value = "CMAKE_STATIC_LINKER_FLAGS", replace = False),
    }

    for key in expected:
        asserts.equals(env, expected[key], res[key])

    return unittest.end(env)

def _merge_toolchain_and_user_values_test(ctx):
    env = unittest.begin(ctx)

    target = {
        "CMAKE_CXX_COMPILER": "$EXT_BUILD_ROOT/external/cxx-value",
        "CMAKE_CXX_FLAGS_INIT": "-ccx-flag",
        "CMAKE_CXX_LINK_EXECUTABLE": "was",
        "CMAKE_C_COMPILER": "some-cc-value",
        "CMAKE_C_FLAGS_INIT": "-cc-flag -gcc_toolchain cc-toolchain",
    }
    source_cache = {
        "CMAKE_ASM_FLAGS": "assemble",
        "CMAKE_CXX_LINK_EXECUTABLE": "became",
        "CMAKE_C_FLAGS": "--additional-flag",
        "CUSTOM": "YES",
    }

    res = export_for_test.merge_toolchain_and_user_values(
        target,
        source_cache,
        export_for_test.CMAKE_CACHE_ENTRIES_CROSSTOOL,
    )

    expected_target = {
        "CMAKE_ASM_FLAGS": "assemble",
        "CMAKE_CXX_FLAGS": "-ccx-flag",
        "CMAKE_CXX_LINK_EXECUTABLE": "became",
        "CMAKE_C_FLAGS": "-cc-flag -gcc_toolchain cc-toolchain --additional-flag",
        "CUSTOM": "YES",
    }

    for key in expected_target:
        asserts.equals(env, expected_target[key], res[key])

    return unittest.end(env)

def _merge_flag_values_no_toolchain_file_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "/usr/bin/gcc",
        cxx = "/usr/bin/gcc",
        cxx_linker_static = "/usr/bin/ar",
        cxx_linker_executable = "/usr/bin/gcc",
    )
    flags = CxxFlagsInfo(
        cc = [],
        cxx = ['foo="bar"'],
        cxx_linker_shared = [],
        cxx_linker_static = [],
        cxx_linker_executable = [],
        assemble = [],
    )
    user_env = {}
    user_cache = {
        "CMAKE_BUILD_TYPE": "RelWithDebInfo",
        "CMAKE_CXX_FLAGS": "-Fbat",
    }

    script = create_cmake_script(
        "ws",
        ctx.label,
        "unknown",
        "unknown",
        "unknown",
        "unknown",
        "Unix Makefiles",
        "cmake",
        tools,
        flags,
        "test_rule",
        "external/test_rule",
        True,
        user_cache,
        user_env,
        [],
        cmake_commands = [],
        cmake_prefix = "emcmake",
    )
    expected = r"""export CC="/usr/bin/gcc"
export CXX="/usr/bin/gcc"
export CXXFLAGS="foo=\\\"bar\\\" -Fbat"
##define_absolute_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_DEPS$$
##define_sandbox_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_ROOT$$
##enable_tracing##
emcmake cmake -DCMAKE_AR="/usr/bin/ar" -DCMAKE_BUILD_TYPE="RelWithDebInfo" -DCMAKE_INSTALL_PREFIX="test_rule" -DCMAKE_PREFIX_PATH="$$EXT_BUILD_DEPS$$" -DPKG_CONFIG_ARGN="--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$" -DCMAKE_RANLIB=""  -G 'Unix Makefiles' $$EXT_BUILD_ROOT$$/external/test_rule
##disable_tracing##
"""
    asserts.equals(env, expected.splitlines(), script)

    return unittest.end(env)

def _create_min_cmake_script_no_toolchain_file_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "/usr/bin/gcc",
        cxx = "/usr/bin/gcc",
        cxx_linker_static = "/usr/bin/ar",
        cxx_linker_executable = "/usr/bin/gcc",
    )
    flags = CxxFlagsInfo(
        cc = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
        cxx = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
        cxx_linker_shared = ["-shared", "-fuse-ld=gold"],
        cxx_linker_static = ["static"],
        cxx_linker_executable = ["-fuse-ld=gold", "-Wl", "-no-as-needed"],
        assemble = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
    )
    user_env = {}
    user_cache = {
        "CMAKE_PREFIX_PATH": "/abc/def",
        "NOFORTRAN": "on",
    }

    script = create_cmake_script(
        "ws",
        ctx.label,
        "unknown",
        "unknown",
        "unknown",
        "unknown",
        "Ninja",
        "cmake",
        tools,
        flags,
        "test_rule",
        "external/test_rule",
        True,
        user_cache,
        user_env,
        ["--debug-output", "-Wdev"],
        cmake_commands = [],
    )
    expected = r"""export CC="/usr/bin/gcc"
export CXX="/usr/bin/gcc"
export CFLAGS="-U_FORTIFY_SOURCE -fstack-protector -Wall"
export CXXFLAGS="-U_FORTIFY_SOURCE -fstack-protector -Wall"
export ASMFLAGS="-U_FORTIFY_SOURCE -fstack-protector -Wall"
##define_absolute_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_DEPS$$
##define_sandbox_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_ROOT$$
##enable_tracing##
cmake -DCMAKE_AR="/usr/bin/ar" -DCMAKE_SHARED_LINKER_FLAGS="-shared -fuse-ld=gold" -DCMAKE_MODULE_LINKER_FLAGS="-shared -fuse-ld=gold" -DCMAKE_EXE_LINKER_FLAGS="-fuse-ld=gold -Wl -no-as-needed" -DNOFORTRAN="on" -DCMAKE_BUILD_TYPE="Debug" -DCMAKE_INSTALL_PREFIX="test_rule" -DCMAKE_PREFIX_PATH="$$EXT_BUILD_DEPS$$;/abc/def" -DPKG_CONFIG_ARGN="--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$" -DCMAKE_RANLIB="" --debug-output -Wdev -G 'Ninja' $$EXT_BUILD_ROOT$$/external/test_rule
##disable_tracing##
"""
    asserts.equals(env, expected.splitlines(), script)

    return unittest.end(env)

def _create_min_cmake_script_wipe_toolchain_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "/usr/bin/gcc",
        cxx = "/usr/bin/gcc",
        cxx_linker_static = "/usr/bin/ar",
        cxx_linker_executable = "/usr/bin/gcc",
    )
    flags = CxxFlagsInfo(
        cc = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
        cxx = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
        cxx_linker_shared = ["-shared", "-fuse-ld=gold"],
        cxx_linker_static = ["static"],
        cxx_linker_executable = ["-fuse-ld=gold", "-Wl", "-no-as-needed"],
        assemble = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
    )
    user_env = {}
    user_cache = {
        "CMAKE_PREFIX_PATH": "/abc/def",
    }
    user_cache.update({
        # These two flags/CMake cache entries must be wiped,
        # but the third is not present in toolchain flags.
        "CMAKE_MODULE_LINKER_FLAGS": "",
        "CMAKE_SHARED_LINKER_FLAGS": "",
        "WIPE_ME_IF_PRESENT": "",
    })

    script = create_cmake_script(
        "ws",
        ctx.label,
        "unknown",
        "unknown",
        "unknown",
        "unknown",
        "Ninja",
        "cmake",
        tools,
        flags,
        "test_rule",
        "external/test_rule",
        True,
        user_cache,
        user_env,
        ["--debug-output", "-Wdev"],
        cmake_commands = [],
    )
    expected = r"""export CC="/usr/bin/gcc"
export CXX="/usr/bin/gcc"
export CFLAGS="-U_FORTIFY_SOURCE -fstack-protector -Wall"
export CXXFLAGS="-U_FORTIFY_SOURCE -fstack-protector -Wall"
export ASMFLAGS="-U_FORTIFY_SOURCE -fstack-protector -Wall"
##define_absolute_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_DEPS$$
##define_sandbox_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_ROOT$$
##enable_tracing##
cmake -DCMAKE_AR="/usr/bin/ar" -DCMAKE_EXE_LINKER_FLAGS="-fuse-ld=gold -Wl -no-as-needed" -DCMAKE_BUILD_TYPE="Debug" -DCMAKE_INSTALL_PREFIX="test_rule" -DCMAKE_PREFIX_PATH="$$EXT_BUILD_DEPS$$;/abc/def" -DPKG_CONFIG_ARGN="--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$" -DCMAKE_RANLIB="" --debug-output -Wdev -G 'Ninja' $$EXT_BUILD_ROOT$$/external/test_rule
##disable_tracing##
"""
    asserts.equals(env, expected.splitlines(), script)

    return unittest.end(env)

def _create_min_cmake_script_toolchain_file_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "/usr/bin/gcc",
        cxx = "/usr/bin/gcc",
        cxx_linker_static = "/usr/bin/ar",
        cxx_linker_executable = "/usr/bin/gcc",
    )
    flags = CxxFlagsInfo(
        cc = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
        cxx = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
        cxx_linker_shared = ["-shared", "-fuse-ld=gold"],
        cxx_linker_static = ["static"],
        cxx_linker_executable = ["-fuse-ld=gold", "-Wl", "-no-as-needed"],
        assemble = ["-U_FORTIFY_SOURCE", "-fstack-protector", "-Wall"],
    )
    user_env = {}
    user_cache = {
        "NOFORTRAN": "on",
    }

    script = create_cmake_script(
        "ws",
        ctx.label,
        "unknown",
        "unknown",
        "unknown",
        "unknown",
        "Ninja",
        "cmake",
        tools,
        flags,
        "test_rule",
        "external/test_rule",
        False,
        user_cache,
        user_env,
        ["--debug-output", "-Wdev"],
        cmake_commands = [],
    )
    expected = r"""__var_CMAKE_AR="/usr/bin/ar"
__var_CMAKE_ASM_FLAGS_INIT="-U_FORTIFY_SOURCE -fstack-protector -Wall"
__var_CMAKE_CXX_COMPILER="/usr/bin/gcc"
__var_CMAKE_CXX_FLAGS_INIT="-U_FORTIFY_SOURCE -fstack-protector -Wall"
__var_CMAKE_C_COMPILER="/usr/bin/gcc"
__var_CMAKE_C_FLAGS_INIT="-U_FORTIFY_SOURCE -fstack-protector -Wall"
__var_CMAKE_EXE_LINKER_FLAGS_INIT="-fuse-ld=gold -Wl -no-as-needed"
__var_CMAKE_MODULE_LINKER_FLAGS_INIT="-shared -fuse-ld=gold"
__var_CMAKE_SHARED_LINKER_FLAGS_INIT="-shared -fuse-ld=gold"
cat > crosstool_bazel.cmake << EOF
set(CMAKE_AR "$$__var_CMAKE_AR$$" CACHE FILEPATH "Archiver")
set(CMAKE_ASM_FLAGS_INIT "$$__var_CMAKE_ASM_FLAGS_INIT$$")
set(CMAKE_CXX_COMPILER "$$__var_CMAKE_CXX_COMPILER$$")
set(CMAKE_CXX_FLAGS_INIT "$$__var_CMAKE_CXX_FLAGS_INIT$$")
set(CMAKE_C_COMPILER "$$__var_CMAKE_C_COMPILER$$")
set(CMAKE_C_FLAGS_INIT "$$__var_CMAKE_C_FLAGS_INIT$$")
set(CMAKE_EXE_LINKER_FLAGS_INIT "$$__var_CMAKE_EXE_LINKER_FLAGS_INIT$$")
set(CMAKE_MODULE_LINKER_FLAGS_INIT "$$__var_CMAKE_MODULE_LINKER_FLAGS_INIT$$")
set(CMAKE_SHARED_LINKER_FLAGS_INIT "$$__var_CMAKE_SHARED_LINKER_FLAGS_INIT$$")
EOF

##define_absolute_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_DEPS$$
##define_sandbox_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_ROOT$$
##enable_tracing##
cmake -DNOFORTRAN="on" -DCMAKE_TOOLCHAIN_FILE="$$BUILD_TMPDIR$$/crosstool_bazel.cmake" -DCMAKE_BUILD_TYPE="Debug" -DCMAKE_INSTALL_PREFIX="test_rule" -DCMAKE_PREFIX_PATH="$$EXT_BUILD_DEPS$$" -DPKG_CONFIG_ARGN="--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$" -DCMAKE_RANLIB="" --debug-output -Wdev -G 'Ninja' $$EXT_BUILD_ROOT$$/external/test_rule
##disable_tracing##
"""
    asserts.equals(env, expected.splitlines(), script)

    return unittest.end(env)

def _create_cmake_script_no_toolchain_file_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "/some-cc-value",
        cxx = "external/cxx-value",
        cxx_linker_static = "/cxx_linker_static",
        cxx_linker_executable = "ws/cxx_linker_executable",
    )
    flags = CxxFlagsInfo(
        cc = ["-cc-flag", "-gcc_toolchain", "cc-toolchain"],
        cxx = [
            "--quoted=\"abc def\"",
            "--sysroot=/abc/sysroot",
            "--gcc_toolchain",
            "cxx-toolchain",
        ],
        cxx_linker_shared = ["shared1", "shared2"],
        cxx_linker_static = ["static"],
        cxx_linker_executable = ["executable"],
        assemble = ["assemble"],
    )
    user_env = {
        "CC": "sink-cc-value",
        "CFLAGS": "--from-env",
        "CUSTOM_ENV": "YES",
        "CXX": "sink-cxx-value",
    }
    user_cache = {
        "CMAKE_ASM_FLAGS": "assemble-user",
        "CMAKE_BUILD_TYPE": "user_type",
        "CMAKE_CXX_LINK_EXECUTABLE": "became",
        "CMAKE_C_FLAGS": "--additional-flag",
        "CUSTOM_CACHE": "YES",
    }

    script = create_cmake_script(
        "ws",
        ctx.label,
        "unknown",
        "unknown",
        "unknown",
        "unknown",
        "Ninja",
        "cmake",
        tools,
        flags,
        "test_rule",
        "external/test_rule",
        True,
        user_cache,
        user_env,
        ["--debug-output", "-Wdev"],
        cmake_commands = [],
    )
    expected = r"""export CC="sink-cc-value"
export CXX="sink-cxx-value"
export CFLAGS="-cc-flag -gcc_toolchain cc-toolchain --from-env --additional-flag"
export CXXFLAGS="--quoted=\\\"abc def\\\" --sysroot=/abc/sysroot --gcc_toolchain cxx-toolchain"
export ASMFLAGS="assemble assemble-user"
export CUSTOM_ENV="YES"
##define_absolute_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_DEPS$$
##define_sandbox_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_ROOT$$
##enable_tracing##
cmake -DCMAKE_AR="/cxx_linker_static" -DCMAKE_CXX_LINK_EXECUTABLE="became" -DCMAKE_SHARED_LINKER_FLAGS="shared1 shared2" -DCMAKE_MODULE_LINKER_FLAGS="shared1 shared2" -DCMAKE_EXE_LINKER_FLAGS="executable" -DCMAKE_BUILD_TYPE="user_type" -DCUSTOM_CACHE="YES" -DCMAKE_INSTALL_PREFIX="test_rule" -DCMAKE_PREFIX_PATH="$$EXT_BUILD_DEPS$$" -DPKG_CONFIG_ARGN="--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$" -DCMAKE_RANLIB="" --debug-output -Wdev -G 'Ninja' $$EXT_BUILD_ROOT$$/external/test_rule
##disable_tracing##
"""
    asserts.equals(env, expected.splitlines(), script)

    return unittest.end(env)

def _create_cmake_script_android_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "/some-cc-value",
        cxx = "external/cxx-value",
        cxx_linker_static = "/cxx_linker_static",
        cxx_linker_executable = "ws/cxx_linker_executable",
    )
    flags = CxxFlagsInfo(
        cc = ["-cc-flag", "-gcc_toolchain", "cc-toolchain"],
        cxx = [
            "--quoted=\"abc def\"",
            "--sysroot=/abc/sysroot",
            "--gcc_toolchain",
            "cxx-toolchain",
        ],
        cxx_linker_shared = ["shared1", "shared2"],
        cxx_linker_static = ["static"],
        cxx_linker_executable = ["executable"],
        assemble = ["assemble"],
    )
    user_env = {
        "CC": "sink-cc-value",
        "CFLAGS": "--from-env",
        "CUSTOM_ENV": "YES",
        "CXX": "sink-cxx-value",
    }
    user_cache = {
        "CMAKE_ASM_FLAGS": "assemble-user",
        "CMAKE_BUILD_TYPE": "user_type",
        "CMAKE_CXX_LINK_EXECUTABLE": "became",
        "CMAKE_C_FLAGS": "--additional-flag",
        "CUSTOM_CACHE": "YES",
    }

    script = create_cmake_script(
        "ws",
        ctx.label,
        "android",
        "x86_64",
        "unknown",
        "unknown",
        "Ninja",
        "cmake",
        tools,
        flags,
        "test_rule",
        "external/test_rule",
        True,
        user_cache,
        user_env,
        ["--debug-output", "-Wdev"],
        cmake_commands = [],
    )
    expected = r"""export CC="sink-cc-value"
export CXX="sink-cxx-value"
export CFLAGS="-cc-flag -gcc_toolchain cc-toolchain --from-env --additional-flag"
export CXXFLAGS="--quoted=\\\"abc def\\\" --sysroot=/abc/sysroot --gcc_toolchain cxx-toolchain"
export ASMFLAGS="assemble assemble-user"
export CUSTOM_ENV="YES"
##define_absolute_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_DEPS$$
##define_sandbox_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_ROOT$$
##enable_tracing##
cmake -DCMAKE_AR="/cxx_linker_static" -DCMAKE_CXX_LINK_EXECUTABLE="became" -DCMAKE_SHARED_LINKER_FLAGS="shared1 shared2" -DCMAKE_MODULE_LINKER_FLAGS="shared1 shared2" -DCMAKE_EXE_LINKER_FLAGS="executable" -DANDROID="YES" -DCMAKE_SYSTEM_NAME="Linux" -DCMAKE_SYSTEM_PROCESSOR="x86_64" -DCMAKE_BUILD_TYPE="user_type" -DCUSTOM_CACHE="YES" -DCMAKE_INSTALL_PREFIX="test_rule" -DCMAKE_PREFIX_PATH="$$EXT_BUILD_DEPS$$" -DPKG_CONFIG_ARGN="--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$" -DCMAKE_RANLIB="" --debug-output -Wdev -G 'Ninja' $$EXT_BUILD_ROOT$$/external/test_rule
##disable_tracing##
"""
    asserts.equals(env, expected.splitlines(), script)

    return unittest.end(env)

def _create_cmake_script_linux_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "/some-cc-value",
        cxx = "external/cxx-value",
        cxx_linker_static = "/cxx_linker_static",
        cxx_linker_executable = "ws/cxx_linker_executable",
    )
    flags = CxxFlagsInfo(
        cc = ["-cc-flag", "-gcc_toolchain", "cc-toolchain"],
        cxx = [
            "--quoted=\"abc def\"",
            "--sysroot=/abc/sysroot",
            "--gcc_toolchain",
            "cxx-toolchain",
        ],
        cxx_linker_shared = ["shared1", "shared2"],
        cxx_linker_static = ["static"],
        cxx_linker_executable = ["executable"],
        assemble = ["assemble"],
    )
    user_env = {
        "CC": "sink-cc-value",
        "CFLAGS": "--from-env",
        "CUSTOM_ENV": "YES",
        "CXX": "sink-cxx-value",
    }
    user_cache = {
        "CMAKE_ASM_FLAGS": "assemble-user",
        "CMAKE_BUILD_TYPE": "user_type",
        "CMAKE_CXX_LINK_EXECUTABLE": "became",
        "CMAKE_C_FLAGS": "--additional-flag",
        "CUSTOM_CACHE": "YES",
    }

    script = create_cmake_script(
        "ws",
        ctx.label,
        "linux",
        "aarch64",
        "unknown",
        "unknown",
        "Ninja",
        "cmake",
        tools,
        flags,
        "test_rule",
        "external/test_rule",
        True,
        user_cache,
        user_env,
        ["--debug-output", "-Wdev"],
        cmake_commands = [],
    )
    expected = r"""export CC="sink-cc-value"
export CXX="sink-cxx-value"
export CFLAGS="-cc-flag -gcc_toolchain cc-toolchain --from-env --additional-flag"
export CXXFLAGS="--quoted=\\\"abc def\\\" --sysroot=/abc/sysroot --gcc_toolchain cxx-toolchain"
export ASMFLAGS="assemble assemble-user"
export CUSTOM_ENV="YES"
##define_absolute_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_DEPS$$
##define_sandbox_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_ROOT$$
##enable_tracing##
cmake -DCMAKE_AR="/cxx_linker_static" -DCMAKE_CXX_LINK_EXECUTABLE="became" -DCMAKE_SHARED_LINKER_FLAGS="shared1 shared2" -DCMAKE_MODULE_LINKER_FLAGS="shared1 shared2" -DCMAKE_EXE_LINKER_FLAGS="executable" -DCMAKE_SYSTEM_NAME="Linux" -DCMAKE_SYSTEM_PROCESSOR="aarch64" -DCMAKE_BUILD_TYPE="user_type" -DCUSTOM_CACHE="YES" -DCMAKE_INSTALL_PREFIX="test_rule" -DCMAKE_PREFIX_PATH="$$EXT_BUILD_DEPS$$" -DPKG_CONFIG_ARGN="--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$" -DCMAKE_RANLIB="" --debug-output -Wdev -G 'Ninja' $$EXT_BUILD_ROOT$$/external/test_rule
##disable_tracing##
"""
    asserts.equals(env, expected.splitlines(), script)

    return unittest.end(env)

def _create_cmake_script_toolchain_file_test(ctx):
    env = unittest.begin(ctx)

    tools = CxxToolsInfo(
        cc = "some-cc-value",
        cxx = "external/cxx-value",
        cxx_linker_static = "/cxx_linker_static",
        cxx_linker_executable = "ws/cxx_linker_executable",
    )
    flags = CxxFlagsInfo(
        cc = ["-cc-flag", "-gcc_toolchain", "cc-toolchain"],
        cxx = [
            "--quoted=\"abc def\"",
            "--sysroot=/abc/sysroot",
            "--gcc_toolchain",
            "cxx-toolchain",
        ],
        cxx_linker_shared = ["shared1", "shared2"],
        cxx_linker_static = ["static"],
        cxx_linker_executable = ["executable"],
        assemble = ["assemble"],
    )
    user_env = {
        "CC": "sink-cc-value",
        "CFLAGS": "--from-env",
        "CUSTOM_ENV": "YES",
        "CXX": "sink-cxx-value",
    }
    user_cache = {
        "CMAKE_ASM_FLAGS": "assemble-user",
        "CMAKE_CXX_LINK_EXECUTABLE": "became",
        "CMAKE_C_FLAGS": "--additional-flag",
        "CUSTOM_CACHE": "YES",
    }

    script = create_cmake_script(
        "ws",
        ctx.label,
        "unknown",
        "unknown",
        "unknown",
        "unknown",
        "Ninja",
        "cmake",
        tools,
        flags,
        "test_rule",
        "external/test_rule",
        False,
        user_cache,
        user_env,
        ["--debug-output", "-Wdev"],
        cmake_commands = [],
    )
    expected = r"""__var_CMAKE_AR="/cxx_linker_static"
__var_CMAKE_ASM_FLAGS_INIT="assemble assemble-user"
__var_CMAKE_CXX_COMPILER="sink-cxx-value"
__var_CMAKE_CXX_COMPILER_EXTERNAL_TOOLCHAIN="cxx-toolchain"
__var_CMAKE_CXX_FLAGS_INIT="--quoted=\\\\\\\"abc def\\\\\\\" --sysroot=/abc/sysroot --gcc_toolchain cxx-toolchain"
__var_CMAKE_CXX_LINK_EXECUTABLE="became"
__var_CMAKE_C_COMPILER="sink-cc-value"
__var_CMAKE_C_COMPILER_EXTERNAL_TOOLCHAIN="cc-toolchain"
__var_CMAKE_C_FLAGS_INIT="-cc-flag -gcc_toolchain cc-toolchain --from-env --additional-flag"
__var_CMAKE_EXE_LINKER_FLAGS_INIT="executable"
__var_CMAKE_MODULE_LINKER_FLAGS_INIT="shared1 shared2"
__var_CMAKE_SHARED_LINKER_FLAGS_INIT="shared1 shared2"
__var_CMAKE_SYSROOT="/abc/sysroot"
cat > crosstool_bazel.cmake << EOF
set(CMAKE_AR "$$__var_CMAKE_AR$$" CACHE FILEPATH "Archiver")
set(CMAKE_ASM_FLAGS_INIT "$$__var_CMAKE_ASM_FLAGS_INIT$$")
set(CMAKE_CXX_COMPILER "$$__var_CMAKE_CXX_COMPILER$$")
set(CMAKE_CXX_COMPILER_EXTERNAL_TOOLCHAIN "$$__var_CMAKE_CXX_COMPILER_EXTERNAL_TOOLCHAIN$$")
set(CMAKE_CXX_FLAGS_INIT "$$__var_CMAKE_CXX_FLAGS_INIT$$")
set(CMAKE_CXX_LINK_EXECUTABLE "$$__var_CMAKE_CXX_LINK_EXECUTABLE$$")
set(CMAKE_C_COMPILER "$$__var_CMAKE_C_COMPILER$$")
set(CMAKE_C_COMPILER_EXTERNAL_TOOLCHAIN "$$__var_CMAKE_C_COMPILER_EXTERNAL_TOOLCHAIN$$")
set(CMAKE_C_FLAGS_INIT "$$__var_CMAKE_C_FLAGS_INIT$$")
set(CMAKE_EXE_LINKER_FLAGS_INIT "$$__var_CMAKE_EXE_LINKER_FLAGS_INIT$$")
set(CMAKE_MODULE_LINKER_FLAGS_INIT "$$__var_CMAKE_MODULE_LINKER_FLAGS_INIT$$")
set(CMAKE_SHARED_LINKER_FLAGS_INIT "$$__var_CMAKE_SHARED_LINKER_FLAGS_INIT$$")
set(CMAKE_SYSROOT "$$__var_CMAKE_SYSROOT$$")
EOF

export CUSTOM_ENV="YES"
##define_absolute_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_DEPS$$
##define_sandbox_paths## $$EXT_BUILD_DEPS$$ $$EXT_BUILD_ROOT$$
##enable_tracing##
cmake -DCUSTOM_CACHE="YES" -DCMAKE_TOOLCHAIN_FILE="$$BUILD_TMPDIR$$/crosstool_bazel.cmake" -DCMAKE_BUILD_TYPE="Debug" -DCMAKE_INSTALL_PREFIX="test_rule" -DCMAKE_PREFIX_PATH="$$EXT_BUILD_DEPS$$" -DPKG_CONFIG_ARGN="--define-variable=EXT_BUILD_DEPS=$$EXT_BUILD_DEPS$$" -DCMAKE_RANLIB="" --debug-output -Wdev -G 'Ninja' $$EXT_BUILD_ROOT$$/external/test_rule
##disable_tracing##
"""
    asserts.equals(env, expected.splitlines(), script)

    return unittest.end(env)

absolutize_test = unittest.make(_absolutize_test)
tail_extraction_test = unittest.make(_tail_extraction_test)
find_flag_value_test = unittest.make(_find_flag_value_test)
fill_crossfile_from_toolchain_test = unittest.make(_fill_crossfile_from_toolchain_test)
move_dict_values_test = unittest.make(_move_dict_values_test)
reverse_descriptor_dict_test = unittest.make(_reverse_descriptor_dict_test)
merge_toolchain_and_user_values_test = unittest.make(_merge_toolchain_and_user_values_test)
create_min_cmake_script_no_toolchain_file_test = unittest.make(_create_min_cmake_script_no_toolchain_file_test)
create_min_cmake_script_toolchain_file_test = unittest.make(_create_min_cmake_script_toolchain_file_test)
create_cmake_script_no_toolchain_file_test = unittest.make(_create_cmake_script_no_toolchain_file_test)
create_cmake_script_toolchain_file_test = unittest.make(_create_cmake_script_toolchain_file_test)
create_cmake_script_android_test = unittest.make(_create_cmake_script_android_test)
create_cmake_script_linux_test = unittest.make(_create_cmake_script_linux_test)
merge_flag_values_no_toolchain_file_test = unittest.make(_merge_flag_values_no_toolchain_file_test)
create_min_cmake_script_wipe_toolchain_test = unittest.make(_create_min_cmake_script_wipe_toolchain_test)

def cmake_script_test_suite():
    unittest.suite(
        "cmake_script_test_suite",
        absolutize_test,
        tail_extraction_test,
        find_flag_value_test,
        fill_crossfile_from_toolchain_test,
        move_dict_values_test,
        reverse_descriptor_dict_test,
        merge_toolchain_and_user_values_test,
        create_min_cmake_script_no_toolchain_file_test,
        create_min_cmake_script_toolchain_file_test,
        create_cmake_script_no_toolchain_file_test,
        create_cmake_script_toolchain_file_test,
        create_cmake_script_android_test,
        create_cmake_script_linux_test,
        merge_flag_values_no_toolchain_file_test,
        create_min_cmake_script_wipe_toolchain_test,
    )
