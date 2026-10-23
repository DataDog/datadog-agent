"""Tests for dd_strip_symbols."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "subjects")
load(":dd_strip.bzl", "dd_strip_symbols")
load(":dd_strip_info.bzl", "DdStripInfo")

def harness(name, impl, config_settings = {}):
    dd_strip_symbols(
        name = name + "_subject",
        src = ":test_binary",
        tags = ["manual"],
    )
    analysis_test(
        name = name,
        impl = impl,
        target = name + "_subject",
        config_settings = config_settings,
    )

def _test_default_output_is_original(name):
    harness(
        name,
        _test_default_output_is_original_impl,
        config_settings = {str(Label("//:install_stripped_binaries")): False},
    )

def _test_default_output_is_original_impl(env, target):
    # //:install_stripped_binaries disabled: packaging installs the unstripped
    # original (the split outputs live under strip_symbols/).
    info = target[DdStripInfo]
    env.expect.that_target(target).default_outputs().contains_exactly([info.packaged_file.short_path])
    env.expect.that_bool("strip_symbols/" in info.packaged_file.short_path).equals(False)

def _test_default_output_is_stripped_when_enabled(name):
    harness(
        name,
        _test_default_output_is_stripped_when_enabled_impl,
        config_settings = {str(Label("//:install_stripped_binaries")): True},
    )

def _test_default_output_is_stripped_when_enabled_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_target(target).default_outputs().contains_exactly([info.packaged_file.short_path])
    stripped_paths = [f.short_path for f in target[OutputGroupInfo].stripped.to_list()]
    env.expect.that_bool(info.packaged_file.short_path in stripped_paths).equals(True)

def _test_dd_strip_info_content(name):
    harness(name, _test_dd_strip_info_content_impl)

def _test_dd_strip_info_content_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_bool(info.packaged_file != None).equals(True)
    env.expect.that_bool(info.debug_file != None).equals(True)

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
            _test_default_output_is_original,
            _test_default_output_is_stripped_when_enabled,
        ],
    )
