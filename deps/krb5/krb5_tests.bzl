"""Wraps krb5's standalone C test programs as manual cc_test targets."""

load("@rules_cc//cc:defs.bzl", "cc_test")

def _krb5_c_tests_impl(name, visibility, srcs, support_srcs, deps, copts, local_defines, data, env):
    tests = []
    for src in srcs:
        test_name = "{}_{}".format(name, src.name.rpartition("/")[2].removesuffix(".c"))
        cc_test(
            name = test_name,
            srcs = [src] + support_srcs,
            copts = copts,
            data = data,
            env = env,
            local_defines = local_defines,
            tags = ["manual"],
            deps = deps,
        )
        tests.append(":" + test_name)
    native.test_suite(
        name = name,
        tags = ["manual"],
        tests = tests,
        visibility = visibility,
    )

krb5_c_tests = macro(
    doc = """Declares one manual cc_test per upstream test program, plus a test_suite named `name` running them all.

    The tests are excluded from wildcards and must be run explicitly, e.g. `bazel test @krb5//:tests`.
    """,
    implementation = _krb5_c_tests_impl,
    attrs = {
        "srcs": attr.label_list(
            allow_files = [".c"],
            mandatory = True,
            configurable = False,
            doc = "Test programs, one cc_test each.",
        ),
        "support_srcs": attr.label_list(
            allow_files = True,
            configurable = False,
            doc = "Helper sources or generated files compiled into every test.",
        ),
        "deps": attr.label_list(doc = "Libraries and private headers the tests need."),
        "copts": attr.string_list(doc = "Compiler flags."),
        "local_defines": attr.string_list(doc = "Preprocessor defines."),
        "data": attr.label_list(allow_files = True, doc = "Runtime data."),
        "env": attr.string_dict(doc = "Environment variables set when running the tests."),
    },
)
