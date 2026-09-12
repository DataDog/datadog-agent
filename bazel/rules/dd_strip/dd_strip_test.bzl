"""Tests for dd_strip_debug and the dd_*_with_debug convenience macros."""

load("@rules_cc//cc:defs.bzl", "cc_binary")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load(":dd_strip.bzl", "dd_cc_binary_with_debug", "dd_strip_debug")
load(":dd_strip_info.bzl", "DdStripInfo")

def _outputs_of(env, target):
    return env.expect.that_target(target).default_outputs()

# Test 1: dd_strip_debug's DefaultInfo always exposes the *original*,
# unstripped file -- wrapping a target must never change what
# `bazel build //x:x` produces by default. The default output must never be
# the .stripped/.debug split artifacts.
def _test_default_output_is_unstripped(name):
    cc_binary(
        name = name + "_bin",
        srcs = ["testdata/main.c"],
    )
    dd_strip_debug(
        name = name + "_subject",
        input = ":" + name + "_bin",
    )
    analysis_test(
        name = name,
        impl = _test_default_output_is_unstripped_impl,
        target = name + "_subject",
    )

def _test_default_output_is_unstripped_impl(env, target):
    info = target[DdStripInfo]
    _outputs_of(env, target).contains_exactly([info.original_file.short_path])

# Test 2: exclude = True passes the original file through unchanged and
# reports DdStripInfo.excluded = True, with no debug_file produced. This
# mirrors omnibus's strip_exclude and must hold on every platform
# (independent of whether a dd_strip toolchain is registered).
def _test_exclude_passthrough(name):
    cc_binary(
        name = name + "_bin",
        srcs = ["testdata/main.c"],
    )
    dd_strip_debug(
        name = name + "_subject",
        input = ":" + name + "_bin",
        exclude = True,
    )
    analysis_test(
        name = name,
        impl = _test_exclude_passthrough_impl,
        target = name + "_subject",
    )

def _test_exclude_passthrough_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_bool(info.excluded).equals(True)
    env.expect.that_bool(info.debug_file == None).equals(True)
    env.expect.that_str(info.stripped_file.path).equals(info.original_file.path)

# Test 3: DdStripInfo is always present, and stripped_file/original_file are
# always set (debug_file may be None when excluded or when no toolchain is
# registered for the current platform -- both are valid outcomes here since
# this test runs on whatever platform is under test).
def _test_dd_strip_info_present(name):
    cc_binary(
        name = name + "_bin",
        srcs = ["testdata/main.c"],
    )
    dd_strip_debug(
        name = name + "_subject",
        input = ":" + name + "_bin",
    )
    analysis_test(
        name = name,
        impl = _test_dd_strip_info_present_impl,
        target = name + "_subject",
    )

def _test_dd_strip_info_present_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_bool(info.stripped_file != None).equals(True)
    env.expect.that_bool(info.original_file != None).equals(True)

    # OutputGroupInfo must always expose both groups (possibly empty),
    # never fail with a missing-group error for consumers that request them.
    groups = target[OutputGroupInfo]
    env.expect.that_bool(hasattr(groups, "stripped")).equals(True)
    env.expect.that_bool(hasattr(groups, "debug")).equals(True)

# Test 4: dd_cc_binary_with_debug creates a private "<name>_unstripped"
# cc_binary and wraps it, forwarding kwargs (e.g. srcs) to the inner binary.
def _test_cc_binary_with_debug_wraps(name):
    dd_cc_binary_with_debug(
        name = name + "_subject",
        srcs = ["testdata/main.c"],
    )
    analysis_test(
        name = name,
        impl = _test_cc_binary_with_debug_wraps_impl,
        target = name + "_subject",
    )

def _test_cc_binary_with_debug_wraps_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_bool(info.original_file != None).equals(True)

# ── Suite ────────────────────────────────────────────────────────────────────

def dd_strip_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_default_output_is_unstripped,
            _test_exclude_passthrough,
            _test_dd_strip_info_present,
            _test_cc_binary_with_debug_wraps,
        ],
    )
