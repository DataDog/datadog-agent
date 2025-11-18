# buildifier: disable=module-docstring
# buildifier: disable=bzl-visibility
load("@rules_foreign_cc//foreign_cc/private:cc_toolchain_util.bzl", "get_flags_info")

def _impl(ctx):
    flags = get_flags_info(ctx)

    assert_contains_once(flags.assemble, "-fblah0")
    assert_contains_once(flags.assemble, "-fblah2")

    assert_contains_once(flags.cc, "-fblah0")
    assert_contains_once(flags.cc, "-fblah2")
    assert_contains_once(flags.cc, "-fblah4")
    if "-fblah5" in flags.cc:
        fail("C flags should not contain '-fblah5'")

    assert_contains_once(flags.cxx, "-fblah0")
    assert_contains_once(flags.cxx, "-fblah1")
    assert_contains_once(flags.cxx, "-fblah4")
    if "-fblah5" in flags.cxx:
        fail("C++ flags should not contain '-fblah5'")

    assert_contains_once(flags.cxx_linker_executable, "-fblah3")
    assert_contains_once(flags.cxx_linker_executable, "-fblah5")
    if "-fblah4" in flags.cxx_linker_executable:
        fail("Executable linker flags should not contain '-fblah4'")

    assert_contains_once(flags.cxx_linker_shared, "-fblah3")
    assert_contains_once(flags.cxx_linker_shared, "-fblah5")
    if "-fblah4" in flags.cxx_linker_shared:
        fail("Shared linker flags should not contain '-fblah4'")

    if "-fblah3" in flags.cxx_linker_static:
        fail("Static linker flags should not contain '-fblah3'")

    exe = ctx.outputs.out
    ctx.actions.write(
        output = exe,
        is_executable = True,
        # The file must not be empty because running an empty .bat file as a
        # subprocess fails on Windows, so we write one space to it.
        content = " ",
    )

    return [DefaultInfo(files = depset([exe]), executable = exe)]

# buildifier: disable=function-docstring
def assert_contains_once(arr, value):
    cnt = 0
    for elem in arr:
        if elem == value:
            cnt = cnt + 1
    if cnt == 0:
        fail("Did not find " + value)
    if cnt > 1:
        fail("Value is included multiple times: " + value)

_flags_test = rule(
    implementation = _impl,
    attrs = {
        "copts": attr.string_list(),
        "deps": attr.label_list(),
        "linkopts": attr.string_list(),
        "out": attr.output(),
        "_cc_toolchain": attr.label(default = Label("@bazel_tools//tools/cpp:current_cc_toolchain")),
    },
    toolchains = ["@bazel_tools//tools/cpp:toolchain_type"],
    fragments = ["cpp", "j2objc"],
    test = True,
)

def flags_test(name, **kwargs):
    _flags_test(
        name = name,
        # On Windows we need the ".bat" extension.
        # On other platforms the extension doesn't matter.
        # Therefore we can use ".bat" on every platform.
        out = name + ".bat",
        **kwargs
    )
