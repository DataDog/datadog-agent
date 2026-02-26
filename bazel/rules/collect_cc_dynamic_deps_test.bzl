"""Tests for the collect_cc_dynamic_deps aspect and cc_dynamic_deps_filegroup rule."""

#load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_testing//lib:util.bzl", "util")
load(":collect_cc_dynamic_deps.bzl", "cc_dynamic_deps_filegroup")

# ── Mock rules ──────────────────────────────────────────────────────────────
# Minimal helper rules that let us exercise the aspect without a C toolchain.

def _fake_file_impl(ctx):
    """Creates a single named file.  Used as a leaf dependency."""
    out = ctx.actions.declare_file(ctx.label.name + ".fake.so")
    ctx.actions.write(out, "")
    return [DefaultInfo(files = depset([out]))]

# A leaf target that simply produces one file.  Does NOT provide
# CcSharedLibraryInfo, so the aspect's 'if CcSharedLibraryInfo in target'
# branch will not fire when visiting it.
_fake_file = rule(implementation = _fake_file_impl)

def _fake_configure_make_impl(ctx):
    """Mimics a rules_foreign_cc configure_make target.

    Has a dynamic_deps attribute but does not provide CcSharedLibraryInfo,
    which exercises the 'elif hasattr(ctx.rule.attr, "dynamic_deps")' branch
    in the aspect.
    """
    out = ctx.actions.declare_file(ctx.label.name + ".fake.so")
    ctx.actions.write(out, "")
    return [DefaultInfo(files = depset([out]))]

_fake_configure_make = rule(
    implementation = _fake_configure_make_impl,
    attrs = {
        "dynamic_deps": attr.label_list(providers = [DefaultInfo]),
    },
)

# ── Test helpers ─────────────────────────────────────────────────────────────

def _outputs_of(env, target):
    return env.expect.that_target(target).default_outputs()

def _pkg(label):
    """Returns the bazel/rules package prefix expected in output paths."""
    return "bazel/rules/" + label

# ── Test 1: target with no dynamic_deps → empty output ─────────────────────

def _test_no_dynamic_deps(name):
    # A plain file target has no dynamic_deps attribute at all.
    _fake_file(name = name + "_leaf")
    util.helper_target(
        cc_dynamic_deps_filegroup,
        name = name + "_subject",
        srcs = [":" + name + "_leaf"],
    )
    analysis_test(
        name = name,
        impl = _test_no_dynamic_deps_impl,
        target = name + "_subject",
    )

def _test_no_dynamic_deps_impl(env, target):
    # Nothing has dynamic_deps, so nothing should be collected.
    _outputs_of(env, target).contains_exactly([])

# ── Test 2: direct dynamic dep → dep's file collected ───────────────────────

def _test_direct_dynamic_dep(name):
    # A configure_make-like target whose single dynamic_dep is a _fake_file.
    _fake_file(name = name + "_dep")
    _fake_configure_make(
        name = name + "_primary",
        dynamic_deps = [":" + name + "_dep"],
    )
    util.helper_target(
        cc_dynamic_deps_filegroup,
        name = name + "_subject",
        srcs = [":" + name + "_primary"],
    )
    analysis_test(
        name = name,
        impl = _test_direct_dynamic_dep_impl,
        target = name + "_subject",
    )

def _test_direct_dynamic_dep_impl(env, target):
    # Exactly one file should be collected: the dynamic dep, not the primary.
    _outputs_of(env, target).contains_exactly([
        _pkg(target.label.name[:-len("_subject")] + "_dep.fake.so"),
    ])

# ── Test 3: aspect propagates through 'srcs' ────────────────────────────────

def _test_propagation_through_srcs(name):
    # Wrap the configure_make-like target in a native filegroup so the aspect
    # must traverse a 'srcs' edge before reaching the dynamic_dep.
    _fake_file(name = name + "_dep")
    _fake_configure_make(
        name = name + "_make",
        dynamic_deps = [":" + name + "_dep"],
    )
    native.filegroup(
        name = name + "_fg",
        srcs = [":" + name + "_make"],
    )
    util.helper_target(
        cc_dynamic_deps_filegroup,
        name = name + "_subject",
        srcs = [":" + name + "_fg"],
    )
    analysis_test(
        name = name,
        impl = _test_propagation_through_srcs_impl,
        target = name + "_subject",
    )

def _test_propagation_through_srcs_impl(env, target):
    # The dep file should bubble up through the filegroup.
    _outputs_of(env, target).contains_exactly([
        _pkg(target.label.name[:-len("_subject")] + "_dep.fake.so"),
    ])

# ── Test 4: transitive dynamic deps ─────────────────────────────────────────
# A has dynamic_deps=[B], B has dynamic_deps=[C].
# Expected collected files: B.fake.so AND C.fake.so.
#   - When the aspect visits A (elif branch): adds B's DefaultInfo.files = {B.fake.so}
#   - When the aspect visits B (elif branch, via attr_aspects traversal): adds C's files = {C.fake.so}
#   - A picks up B's _TransitiveCcDynamicDepsInfo.libs = {C.fake.so} transitively.
# Total: 2 files.

def _test_transitive_collection(name):
    _fake_file(name = name + "_c")
    _fake_configure_make(
        name = name + "_b",
        dynamic_deps = [":" + name + "_c"],
    )
    _fake_configure_make(
        name = name + "_a",
        dynamic_deps = [":" + name + "_b"],
    )
    util.helper_target(
        cc_dynamic_deps_filegroup,
        name = name + "_subject",
        srcs = [":" + name + "_a"],
    )
    analysis_test(
        name = name,
        impl = _test_transitive_collection_impl,
        target = name + "_subject",
    )

def _test_transitive_collection_impl(env, target):
    prefix = target.label.name[:-len("_subject")]

    # B's own file is collected because A depends on B dynamically.
    # C's file is collected because B depends on C dynamically.
    _outputs_of(env, target).contains_exactly([
        _pkg(prefix + "_b.fake.so"),
        _pkg(prefix + "_c.fake.so"),
    ])

# ── Test 5: multiple dynamic_deps on a single target ────────────────────────

def _test_multiple_direct_dynamic_deps(name):
    _fake_file(name = name + "_dep1")
    _fake_file(name = name + "_dep2")
    _fake_configure_make(
        name = name + "_primary",
        dynamic_deps = [
            ":" + name + "_dep1",
            ":" + name + "_dep2",
        ],
    )
    util.helper_target(
        cc_dynamic_deps_filegroup,
        name = name + "_subject",
        srcs = [":" + name + "_primary"],
    )
    analysis_test(
        name = name,
        impl = _test_multiple_direct_dynamic_deps_impl,
        target = name + "_subject",
    )

def _test_multiple_direct_dynamic_deps_impl(env, target):
    prefix = target.label.name[:-len("_subject")]
    _outputs_of(env, target).contains_exactly([
        _pkg(prefix + "_dep1.fake.so"),
        _pkg(prefix + "_dep2.fake.so"),
    ])

# ── Test 6: diamond deduplication ───────────────────────────────────────────
# A has dynamic_deps=[B, C]; both B and C have dynamic_deps=[D].
# D should appear only once in the output.

def _test_diamond_deduplication(name):
    _fake_file(name = name + "_d")
    _fake_configure_make(
        name = name + "_b",
        dynamic_deps = [":" + name + "_d"],
    )
    _fake_configure_make(
        name = name + "_c",
        dynamic_deps = [":" + name + "_d"],
    )
    _fake_configure_make(
        name = name + "_a",
        dynamic_deps = [
            ":" + name + "_b",
            ":" + name + "_c",
        ],
    )
    util.helper_target(
        cc_dynamic_deps_filegroup,
        name = name + "_subject",
        srcs = [":" + name + "_a"],
    )
    analysis_test(
        name = name,
        impl = _test_diamond_deduplication_impl,
        target = name + "_subject",
    )

def _test_diamond_deduplication_impl(env, target):
    prefix = target.label.name[:-len("_subject")]

    # B, C, and D are all reachable; D must not be duplicated.
    _outputs_of(env, target).contains_exactly([
        _pkg(prefix + "_b.fake.so"),
        _pkg(prefix + "_c.fake.so"),
        _pkg(prefix + "_d.fake.so"),
    ])

# ── Test 7: CcSharedLibraryInfo branch ──────────────────────────────────────
# Uses real cc_shared_library rules to exercise the
# 'if CcSharedLibraryInfo in target' branch.
#
# Topology:
#   cc_shared_library "outer" has dynamic_deps=[cc_shared_library "inner"]
#   cc_dynamic_deps_filegroup applied to "outer"
#   Expected: "inner"'s .so file is in the output; "outer"'s .so is NOT.

def _test_cc_shared_library_dynamic_deps(name):
    # Inner shared library (the runtime dependency).
    # target_compatible_with propagates: the analysis_test is automatically
    # skipped on platforms where the cc toolchain is not available.
    native.cc_library(
        name = name + "_inner_lib",
        srcs = ["testdata/empty.c"],
        target_compatible_with = ["@platforms//os:linux"],
    )
    native.cc_shared_library(
        name = name + "_inner",
        deps = [":" + name + "_inner_lib"],
        target_compatible_with = ["@platforms//os:linux"],
    )

    # Outer shared library that depends on inner at runtime.
    native.cc_library(
        name = name + "_outer_lib",
        srcs = ["testdata/empty.c"],
        target_compatible_with = ["@platforms//os:linux"],
    )
    native.cc_shared_library(
        name = name + "_outer",
        deps = [":" + name + "_outer_lib"],
        dynamic_deps = [":" + name + "_inner"],
        target_compatible_with = ["@platforms//os:linux"],
    )

    util.helper_target(
        cc_dynamic_deps_filegroup,
        name = name + "_subject",
        srcs = [":" + name + "_outer"],
    )
    analysis_test(
        name = name,
        impl = _test_cc_shared_library_dynamic_deps_impl,
        target = name + "_subject",
    )

def _test_cc_shared_library_dynamic_deps_impl(env, target):
    # The inner library's .so should be collected; the outer library must NOT
    # appear (its caller is responsible for packaging it directly).
    #
    # cc_shared_library places its output under _solib_local/<mangled-pkg>/
    # rather than in the package directory, so we match on the file basename
    # rather than the full path to stay robust against CPU-config changes.
    prefix = target.label.name[:-len("_subject")]
    outputs = _outputs_of(env, target)
    outputs.contains_predicate(matching.file_basename_contains("lib" + prefix + "_inner.so"))
    outputs.not_contains_predicate(matching.file_basename_contains(prefix + "_outer"))

# ── Suite ────────────────────────────────────────────────────────────────────

def collect_cc_dynamic_deps_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_no_dynamic_deps,
            _test_direct_dynamic_dep,
            _test_propagation_through_srcs,
            _test_transitive_collection,
            _test_multiple_direct_dynamic_deps,
            _test_diamond_deduplication,
            _test_cc_shared_library_dynamic_deps,
        ],
    )
