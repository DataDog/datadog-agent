"""Aspect to automatically collect cc_shared_library files from dynamic_deps.

When building a distribution package, shared libraries that are listed as
dynamic_deps of cc_shared_library (or configure_make) targets are runtime
dependencies that must be shipped alongside the primary artifacts. Manually
tracking these transitive dependencies is error-prone and tedious.

This module provides an aspect + rule pair that traverses the dependency graph
and automatically gathers those files, removing the need to list them by hand.

Typical usage:

    cc_dynamic_deps_filegroup(
        name = "transitive_shared_libs",
        srcs = [":my_pkg_filegroup"],
    )

    pkg_files(
        name = "transitive_shared_libs_pkg",
        srcs = [":transitive_shared_libs"],
        prefix = "embedded/lib",
    )

    pkg_filegroup(
        name = "transitive_shared_libs_with_prefix",
        srcs = [":transitive_shared_libs_pkg"],
        prefix = "/opt/datadog-agent",
    )
"""

load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")

_TransitiveCcDynamicDepsInfo = provider(
    doc = """Accumulates cc_shared_library files reached through dynamic_deps edges.

Propagated by the _collect_cc_dynamic_deps aspect.  Each target in the
build graph contributes the shared-library files that its subtree exposes
through dynamic_deps relationships.""",
    fields = {
        "libs": "depset of File objects — the .so/.dylib/.dll files collected from dynamic_deps",
    },
)

def _collect_cc_dynamic_deps_impl(target, ctx):
    transitive = []
    direct = []

    # Propagate the provider upward through every attribute we traverse.
    for attr_name in ["srcs", "deps", "dynamic_deps"]:
        if hasattr(ctx.rule.attr, attr_name):
            attr_value = getattr(ctx.rule.attr, attr_name)

            # All three attributes are label_list in the rules we care about,
            # but guard against single-label or non-list values just in case.
            if type(attr_value) != "list":
                continue

            for dep in attr_value:
                if _TransitiveCcDynamicDepsInfo in dep:
                    transitive.append(dep[_TransitiveCcDynamicDepsInfo].libs)

    if CcSharedLibraryInfo in target:
        # Native cc_shared_library targets expose CcSharedLibraryInfo whose
        # `dynamic_deps` field is the *transitive closure* of all dynamic
        # dependencies (Bazel keeps it up-to-date automatically).  Extract
        # the shared library file from each entry.
        #
        # Note: we deliberately do NOT add the target's own files here — the
        # caller (pkg_filegroup / all_files) is expected to package the target
        # itself.  We only surface what it *depends on* at runtime.
        for dep_info in target[CcSharedLibraryInfo].dynamic_deps.to_list():
            for lib in dep_info.linker_input.libraries:
                if lib.dynamic_library != None:
                    direct.append(lib.dynamic_library)

    elif hasattr(ctx.rule.attr, "dynamic_deps"):
        # Non-cc_shared_library rules that carry a dynamic_deps attribute
        # (e.g. rules_foreign_cc's configure_make) do not provide
        # CcSharedLibraryInfo, so we cannot use the transitive-closure
        # shortcut above.  Instead, collect the DefaultInfo.files of each
        # direct dynamic dep.  Their own transitive dynamic_deps will be
        # picked up when the aspect visits *those* targets (via the
        # attr_aspects traversal).
        attr_value = ctx.rule.attr.dynamic_deps
        if type(attr_value) == "list":
            for dep in attr_value:
                if DefaultInfo in dep:
                    direct.extend(dep[DefaultInfo].files.to_list())

    return [_TransitiveCcDynamicDepsInfo(libs = depset(direct, transitive = transitive))]

_collect_cc_dynamic_deps = aspect(
    implementation = _collect_cc_dynamic_deps_impl,
    # Traverse all three attribute families so we reach cc_shared_library
    # targets that live deep inside pkg_filegroup/pkg_files hierarchies as
    # well as targets that are themselves listed as dynamic_deps.
    attr_aspects = ["srcs", "deps", "dynamic_deps"],
    provides = [_TransitiveCcDynamicDepsInfo],
    doc = """\
Traverses the dependency graph and collects the shared-library files (.so /
.dylib / .dll) that are exposed through dynamic_deps relationships.

Apply this aspect (via cc_dynamic_deps_filegroup) to a top-level packaging
target such as a pkg_filegroup.  The aspect walks the full subgraph and, for
every cc_shared_library it encounters, extracts the files from its transitive
dynamic_deps closure.  For configure_make (or similar) targets that carry a
dynamic_deps attribute without providing CcSharedLibraryInfo, the direct
dynamic dep files are collected instead.""",
)

def _cc_dynamic_deps_filegroup_impl(ctx):
    libs = depset(transitive = [
        dep[_TransitiveCcDynamicDepsInfo].libs
        for dep in ctx.attr.srcs
        if _TransitiveCcDynamicDepsInfo in dep
    ])
    return [DefaultInfo(files = libs)]

cc_dynamic_deps_filegroup = rule(
    implementation = _cc_dynamic_deps_filegroup_impl,
    attrs = {
        "srcs": attr.label_list(
            aspects = [_collect_cc_dynamic_deps],
            doc = """\
Top-level targets whose dependency subgraph should be searched for
dynamic_deps.  Typically these are pkg_filegroup targets representing the
explicitly-packaged artifacts of a distribution.""",
        ),
    },
    doc = """\
Collects all cc_shared_library files that appear as dynamic_deps (directly
or transitively) of the targets listed in `srcs`.

The result is a filegroup (DefaultInfo) containing the raw shared-library
files.  Wrap it in a pkg_files rule to place the files in the correct
installation directory:

    cc_dynamic_deps_filegroup(
        name = "transitive_shared_libs",
        srcs = [":agent_components"],
    )

    pkg_files(
        name = "transitive_shared_libs_pkg",
        srcs = [":transitive_shared_libs"],
        prefix = "embedded/lib",
    )
""",
)
