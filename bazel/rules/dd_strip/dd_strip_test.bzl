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

# Tree inputs are only splittable on platforms that provide a tree_driver
# (Linux); elsewhere dd_strip_symbols passes trees through unchanged. The
# :test_tree fixture is linux-only, which also skips these tests elsewhere.
def harness_tree(name, impl, config_settings = {}):
    dd_strip_symbols(
        name = name + "_subject",
        src = ":test_tree",
        tags = ["manual"],
        target_compatible_with = ["@platforms//os:linux"],
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

def _test_tree_default_output_is_original(name):
    harness_tree(
        name,
        _test_tree_default_output_is_original_impl,
        config_settings = {str(Label("//:install_stripped_binaries")): False},
    )

def _test_tree_default_output_is_original_impl(env, target):
    # //:install_stripped_binaries disabled: packaging installs the unstripped
    # original tree (the split outputs live under strip_symbols/).
    info = target[DdStripInfo]
    env.expect.that_target(target).default_outputs().contains_exactly([info.packaged_file.short_path])
    env.expect.that_bool("strip_symbols/" in info.packaged_file.short_path).equals(False)
    env.expect.that_bool(info.packaged_file.is_directory).equals(True)

def _test_tree_default_output_is_stripped_when_enabled(name):
    harness_tree(
        name,
        _test_tree_default_output_is_stripped_when_enabled_impl,
        config_settings = {str(Label("//:install_stripped_binaries")): True},
    )

def _test_tree_default_output_is_stripped_when_enabled_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_target(target).default_outputs().contains_exactly([info.packaged_file.short_path])
    stripped_paths = [f.short_path for f in target[OutputGroupInfo].stripped.to_list()]
    env.expect.that_bool(info.packaged_file.short_path in stripped_paths).equals(True)
    env.expect.that_bool(info.packaged_file.is_directory).equals(True)

def _test_tree_dd_strip_info_content(name):
    harness_tree(name, _test_tree_dd_strip_info_content_impl)

def _test_tree_dd_strip_info_content_impl(env, target):
    info = target[DdStripInfo]
    env.expect.that_bool(info.packaged_file != None).equals(True)
    env.expect.that_bool(info.debug_file != None).equals(True)

    # The debug twin of a tree mirrors the packaged layout: it is a
    # TreeArtifact whose contents carry one .dbg per ELF at relpath + .dbg,
    # placed at the original's destination directory.
    env.expect.that_bool(info.debug_file.is_directory).equals(True)

    env.expect.that_value(
        info.original,
        factory = subjects.label,
        expr = "label",
    ).equals(Label("//bazel/rules/dd_strip:test_tree"))

    groups = target[OutputGroupInfo]
    env.expect.that_bool(hasattr(groups, "stripped")).equals(True)
    env.expect.that_bool(hasattr(groups, "debug")).equals(True)
    env.expect.that_collection([f.is_directory for f in groups.stripped.to_list()]).contains_exactly([True])
    env.expect.that_collection([f.is_directory for f in groups.debug.to_list()]).contains_exactly([True])

def dd_strip_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_dd_strip_info_content,
            _test_default_output_is_original,
            _test_default_output_is_stripped_when_enabled,
            _test_tree_dd_strip_info_content,
            _test_tree_default_output_is_original,
            _test_tree_default_output_is_stripped_when_enabled,
        ],
    )
