"""Tests for dd_strip_debug."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "subjects")
load(":dd_strip.bzl", "dd_strip_symbols")
load(":dd_strip_info.bzl", "DdStripInfo")

def harness(name, impl):
    dd_strip_symbols(
        name = name + "_subject",
        src = ":test_binary",
        tags = ["manual"],
    )
    analysis_test(
        name = name,
        impl = impl,
        target = name + "_subject",
    )

    # we do not need these for the analysis tests. They are here to make it
    # easy to build the various outputs and hand verify.
    native.filegroup(
        name = name + "_stripped",
        srcs = [name + "_subject"],
        output_group = "stripped",
        tags = ["manual"],
    )
    native.filegroup(
        name = name + "_debug",
        srcs = [name + "_subject"],
        output_group = "debug",
        tags = ["manual"],
    )

def _test_default_output_is_stripped(name):
    harness(name, _test_default_output_is_stripped_impl)

def _test_default_output_is_stripped_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_target(target).default_outputs().contains_exactly([info.stripped_file.short_path])

def _test_dd_strip_info_content(name):
    harness(name, _test_dd_strip_info_content_impl)

def _test_dd_strip_info_content_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_bool(info.stripped_file != None).equals(True)
    env.expect.that_bool(info.original_file != None).equals(True)

    subjects.label(info.original, env.expect.meta).equals(Label("//bazel/rules/dd_strip:test_binary"))

    # OutputGroupInfo must always expose both groups (possibly empty),
    # never fail with a missing-group error for consumers that request them.
    groups = target[OutputGroupInfo]
    env.expect.that_bool(hasattr(groups, "stripped")).equals(True)
    env.expect.that_bool(hasattr(groups, "debug")).equals(True)

# ── Suite ────────────────────────────────────────────────────────────────────

def dd_strip_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_dd_strip_info_content,
            _test_default_output_is_stripped,
        ],
    )
