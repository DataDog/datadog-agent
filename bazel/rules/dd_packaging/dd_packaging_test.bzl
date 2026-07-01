"""Tests for dd_collect_dependencies and dd_cc_packaged."""

load("@rules_cc//cc:defs.bzl", "cc_binary", "cc_library", "cc_shared_library")
load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")
load("@rules_pkg//pkg:mappings.bzl", "pkg_files")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_testing//lib:util.bzl", "util")
load("//bazel/rules/dd_packaging:dd_cc_packaged.bzl", "dd_cc_packaged")
load("//bazel/rules/dd_packaging:dd_collect_dependencies.bzl", "dd_collect_dependencies")
load("//bazel/rules/dd_packaging:dd_packaging_info.bzl", "DdPackagingInfo")

# Helper rule that extracts the dynamic libraries from CcSharedLibraryInfo
# into DefaultInfo so analysis tests can inspect them.
def _cc_shared_library_dynamic_libs_impl(ctx):
    libs = ctx.attr.dep[CcSharedLibraryInfo].linker_input.libraries
    files = [lib.dynamic_library for lib in libs if lib.dynamic_library != None]
    return DefaultInfo(files = depset(files))

_cc_shared_library_dynamic_libs = rule(
    implementation = _cc_shared_library_dynamic_libs_impl,
    attrs = {"dep": attr.label(providers = [CcSharedLibraryInfo])},
)

# Helper rule that flattens DdPackagingInfo.installed_files into DefaultInfo
# so analysis tests can inspect the actual source files being packaged.
def _dd_packaging_installed_files_impl(ctx):
    files = []
    for filegroup in ctx.attr.dep[DdPackagingInfo].installed_files:
        for pkg_files_info, _ in filegroup.pkg_files:
            files.extend(pkg_files_info.dest_src_map.values())
    return DefaultInfo(files = depset(files))

_dd_packaging_installed_files = rule(
    implementation = _dd_packaging_installed_files_impl,
    attrs = {"dep": attr.label(providers = [DdPackagingInfo])},
)

def _outputs_of(env, target):
    return env.expect.that_target(target).default_outputs()

# Helper rule that inspects PackageFilegroupInfo.pkg_files for duplicate
# destinations.  Produces a marker file if any destination appears more than
# once so analysis tests can assert the output is empty (no duplicates).
def _duplicate_destinations_impl(ctx):
    pfgi = ctx.attr.dep[PackageFilegroupInfo]
    seen = {}
    for pkg_files_info, _ in pfgi.pkg_files:
        for dest in pkg_files_info.dest_src_map:
            if dest in seen:
                marker = ctx.actions.declare_file(ctx.label.name + "_duplicate_detected")
                ctx.actions.write(marker, dest)
                return DefaultInfo(files = depset([marker]))
            seen[dest] = True
    return DefaultInfo(files = depset([]))

_duplicate_destinations = rule(
    implementation = _duplicate_destinations_impl,
    attrs = {"dep": attr.label(providers = [PackageFilegroupInfo])},
)

# ── Test cases ───────────────────────────────────────────────────────────────

# Test 1: a cc_shared_library without a dd_cc_packaged wrapper has no
# DdPackagingInfo, so dd_collect_dependencies produces empty output.
def _test_no_packaging_info(name):
    cc_library(
        name = name + "_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_so",
        deps = [":" + name + "_lib"],
    )
    util.helper_target(
        dd_collect_dependencies,
        name = name + "_subject",
        srcs = [":" + name + "_so"],
    )
    analysis_test(
        name = name,
        impl = _test_no_packaging_info_impl,
        target = name + "_subject",
    )

def _test_no_packaging_info_impl(env, target):
    _outputs_of(env, target).contains_exactly([])

# Test 2: dd_cc_packaged wrapping a cc_shared_library → the rpath-patched .so
# appears in the dd_collect_dependencies output.
def _test_so_collected(name):
    cc_library(
        name = name + "_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_so",
        deps = [":" + name + "_lib"],
    )
    dd_cc_packaged(
        name = name + "_packaged",
        input = ":" + name + "_so",
    )
    util.helper_target(
        dd_collect_dependencies,
        name = name + "_subject",
        srcs = [":" + name + "_packaged"],
    )
    analysis_test(
        name = name,
        impl = _test_so_collected_impl,
        target = name + "_subject",
    )

def _test_so_collected_impl(env, target):
    # rewrite_rpath places its output under a "patched/" subdirectory.
    _outputs_of(env, target).contains_predicate(
        matching.file_path_matches("*patched/*"),
    )

# Test 3: dd_cc_packaged with installed_files → both the patched .so and the
# extra installed files (headers) appear in the output.
def _test_installed_files_collected(name):
    cc_library(
        name = name + "_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_so",
        deps = [":" + name + "_lib"],
    )
    pkg_files(
        name = name + "_hdrs",
        srcs = ["testdata/empty.h"],
        prefix = "include",
    )
    dd_cc_packaged(
        name = name + "_packaged",
        input = ":" + name + "_so",
        installed_files = [":" + name + "_hdrs"],
    )
    util.helper_target(
        dd_collect_dependencies,
        name = name + "_subject",
        srcs = [":" + name + "_packaged"],
    )
    analysis_test(
        name = name,
        impl = _test_installed_files_collected_impl,
        target = name + "_subject",
    )

def _test_installed_files_collected_impl(env, target):
    outputs = _outputs_of(env, target)
    outputs.contains_predicate(matching.file_path_matches("*patched/*"))
    outputs.contains_predicate(matching.file_basename_contains("empty.h"))

# Test 4: transitive collection through dynamic_deps.
#
# outer_so --[dynamic_deps]--> inner_so
#
# inner_so carries a header in its installed_files.  Collecting from outer_so
# must surface that header even though outer_so does not directly reference it.
def _test_transitive_collected(name):
    cc_library(
        name = name + "_inner_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_inner_so",
        deps = [":" + name + "_inner_lib"],
    )
    pkg_files(
        name = name + "_inner_hdrs",
        srcs = ["testdata/empty.h"],
        prefix = "include",
    )
    dd_cc_packaged(
        name = name + "_inner_packaged",
        input = ":" + name + "_inner_so",
        installed_files = [":" + name + "_inner_hdrs"],
    )
    cc_library(
        name = name + "_outer_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_outer_so",
        deps = [":" + name + "_outer_lib"],
        dynamic_deps = [":" + name + "_inner_packaged"],
    )
    dd_cc_packaged(
        name = name + "_outer_packaged",
        input = ":" + name + "_outer_so",
    )
    util.helper_target(
        dd_collect_dependencies,
        name = name + "_subject",
        srcs = [":" + name + "_outer_packaged"],
    )
    analysis_test(
        name = name,
        impl = _test_transitive_collected_impl,
        target = name + "_subject",
    )

def _test_transitive_collected_impl(env, target):
    outputs = _outputs_of(env, target)

    # outer's own patched .so
    outputs.contains_predicate(matching.file_path_matches("*patched/*"))

    # inner's header, reached transitively via dynamic_deps → input → dynamic_deps
    outputs.contains_predicate(matching.file_basename_contains("empty.h"))

# Test 5: dd_cc_packaged itself has no default outputs.
#
# The {name}_patched and {name}_packaged side-targets must NOT be built when
# another target simply depends on the dd_cc_packaged rule.  They are only
# materialised at package time, when dd_collect_dependencies explicitly
# collects them.
def _test_packaged_has_no_build_outputs(name):
    cc_library(
        name = name + "_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_so",
        deps = [":" + name + "_lib"],
    )
    dd_cc_packaged(
        name = name + "_packaged",
        input = ":" + name + "_so",
    )
    analysis_test(
        name = name,
        impl = _test_packaged_has_no_build_outputs_impl,
        target = name + "_packaged",
    )

def _test_packaged_has_no_build_outputs_impl(env, target):
    _outputs_of(env, target).contains_exactly([])

# Test 6: dd_cc_packaged forwards the unpatched CcSharedLibraryInfo.
#
# When another target depends on name_packaged as a dynamic_dep, it must
# receive the original (unpatched) .so, not the rpath-patched copy.  The
# patched copy is only materialised at package time.
def _test_packaged_forwards_unpatched_so(name):
    cc_library(
        name = name + "_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_so",
        deps = [":" + name + "_lib"],
    )
    dd_cc_packaged(
        name = name + "_packaged",
        input = ":" + name + "_so",
    )
    util.helper_target(
        _cc_shared_library_dynamic_libs,
        name = name + "_subject",
        dep = ":" + name + "_packaged",
    )
    analysis_test(
        name = name,
        impl = _test_packaged_forwards_unpatched_so_impl,
        target = name + "_subject",
    )

def _test_packaged_forwards_unpatched_so_impl(env, target):
    outputs = _outputs_of(env, target)
    outputs.contains_predicate(matching.file_extension_in(["so", "dll", "dylib"]))
    outputs.not_contains_predicate(matching.file_path_matches("*patched/*"))

# Test 7: dd_cc_packaged wrapping a cc_binary → the rpath-patched binary
# appears in DdPackagingInfo.installed_files.
#
# Note: unlike cc_shared_library, a packaged cc_binary exposes no CC provider,
# so it cannot appear in dd_collect_dependencies.srcs.  The packaged binary is
# a top-level artifact collected directly via its DdPackagingInfo.
def _test_cc_binary_collected(name):
    cc_binary(
        name = name + "_bin",
        srcs = ["testdata/main.c"],
    )
    dd_cc_packaged(
        name = name + "_packaged",
        input = ":" + name + "_bin",
    )
    util.helper_target(
        _dd_packaging_installed_files,
        name = name + "_subject",
        dep = ":" + name + "_packaged",
    )
    analysis_test(
        name = name,
        impl = _test_cc_binary_collected_impl,
        target = name + "_subject",
    )

def _test_cc_binary_collected_impl(env, target):
    _outputs_of(env, target).contains_predicate(matching.file_path_matches("*patched/*"))

# Test 8: dd_cc_packaged wrapping a cc_binary does NOT forward CcSharedLibraryInfo,
# even when the binary links against a dd_cc_packaged shared library.
#
# The inner shared library correctly exposes CcSharedLibraryInfo, but that
# must not leak through the binary wrapper — consumers of the binary cannot
# use it as a dynamic_dep.
def _test_cc_binary_no_cc_shared_library_info(name):
    cc_library(
        name = name + "_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_so",
        deps = [":" + name + "_lib"],
    )
    dd_cc_packaged(
        name = name + "_so_packaged",
        input = ":" + name + "_so",
    )
    cc_binary(
        name = name + "_bin",
        srcs = ["testdata/main.c"],
        dynamic_deps = [":" + name + "_so_packaged"],
    )
    dd_cc_packaged(
        name = name + "_packaged",
        input = ":" + name + "_bin",
    )
    analysis_test(
        name = name,
        impl = _test_cc_binary_no_cc_shared_library_info_impl,
        target = name + "_packaged",
    )

def _test_cc_binary_no_cc_shared_library_info_impl(env, target):
    env.expect.that_bool(CcSharedLibraryInfo in target).equals(False)

# Test 9: diamond dependency — shared_so is reachable via two independent
# paths; its installed files must appear exactly once in the merged output.
#
#   top_so --[dynamic_deps]--> left_so  --[dynamic_deps]--> shared_so
#                          \-> right_so --[dynamic_deps]--> shared_so
#
# Without depset-based deduplication in _CollectedPackagingInfo the same
# PackageFilegroupInfo would be accumulated twice, triggering a duplicate-
# destination error at pkg_rpm / pkg_deb time.
def _test_diamond_no_duplicates(name):
    cc_library(
        name = name + "_shared_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_shared_so",
        deps = [":" + name + "_shared_lib"],
    )
    pkg_files(
        name = name + "_shared_hdrs",
        srcs = ["testdata/empty.h"],
        prefix = "include",
    )
    dd_cc_packaged(
        name = name + "_shared_packaged",
        input = ":" + name + "_shared_so",
        installed_files = [":" + name + "_shared_hdrs"],
    )

    cc_library(
        name = name + "_left_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_left_so",
        deps = [":" + name + "_left_lib"],
        dynamic_deps = [":" + name + "_shared_packaged"],
    )
    dd_cc_packaged(
        name = name + "_left_packaged",
        input = ":" + name + "_left_so",
    )

    cc_library(
        name = name + "_right_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_right_so",
        deps = [":" + name + "_right_lib"],
        dynamic_deps = [":" + name + "_shared_packaged"],
    )
    dd_cc_packaged(
        name = name + "_right_packaged",
        input = ":" + name + "_right_so",
    )

    cc_library(
        name = name + "_top_lib",
        srcs = ["testdata/empty.c"],
    )
    cc_shared_library(
        name = name + "_top_so",
        deps = [":" + name + "_top_lib"],
        dynamic_deps = [
            ":" + name + "_left_packaged",
            ":" + name + "_right_packaged",
        ],
    )
    dd_cc_packaged(
        name = name + "_top_packaged",
        input = ":" + name + "_top_so",
    )
    util.helper_target(
        dd_collect_dependencies,
        name = name + "_collected",
        srcs = [":" + name + "_top_packaged"],
    )
    util.helper_target(
        _duplicate_destinations,
        name = name + "_subject",
        dep = ":" + name + "_collected",
    )
    analysis_test(
        name = name,
        impl = _test_diamond_no_duplicates_impl,
        target = name + "_subject",
    )

def _test_diamond_no_duplicates_impl(env, target):
    _outputs_of(env, target).contains_exactly([])

# ── Suite ────────────────────────────────────────────────────────────────────

def dd_packaging_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_no_packaging_info,
            _test_so_collected,
            _test_installed_files_collected,
            _test_transitive_collected,
            _test_packaged_has_no_build_outputs,
            _test_packaged_forwards_unpatched_so,
            _test_cc_binary_collected,
            _test_cc_binary_no_cc_shared_library_info,
            _test_diamond_no_duplicates,
        ],
    )
