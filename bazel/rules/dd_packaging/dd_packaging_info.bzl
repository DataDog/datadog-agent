"""DdPackagingInfo — common provider for the DD packaging decorator chain.

Every rule in the chain (dd_cc_shared_library, dd_cc_binary, dd_so_symlink, …)
reads and/or produces this provider.  It carries two distinct depsets so that
decorators can override the direct representation of a target without losing the
transitive context accumulated by earlier chain members.

Typical flow
------------

    cc_shared_library(:libfoo)
        ↓
    dd_cc_shared_library(:libfoo_dd)
        direct_files            = depset([patched_libfoo.so])
        transitive_deps_files   = depset(transitive deps' all_files)
        ↓
    dd_so_symlink(:libfoo_sym)
        direct_files            = depset([patched_libfoo.so, libfoo.so.1, libfoo.so.1.2.3])
        transitive_deps_files   = forwarded unchanged from :libfoo_dd

The collection rule/aspect reads `all_files` (the union of both depsets) from
the outermost decorator at each dependency edge.
"""

DdPackagingInfo = provider(
    doc = """\
Carries the set of files that should be installed for a packaged target.

Produced by dd_cc_shared_library, dd_cc_binary, and every subsequent decorator
(dd_so_symlink, …).  Each rule in the chain may replace `direct_files` while
forwarding `transitive_deps_files` unchanged, so that the outermost decorator's
`all_files` always represents the complete, correct file set for that subtree.
""",
    fields = {
        "direct_file": """\
File | None.  The single primary output of this rule — the patched .so (or
patched binary for dd_cc_binary).  Set by dd_cc_shared_library /
dd_cc_binary and forwarded unchanged by decorators so that downstream
rules (e.g. dd_so_symlink) can locate the file without iterating a depset.
""",
        "direct_files": """\
depset[File].  The packaging files *owned by this rule instance*.  For
dd_cc_shared_library this is just the patched .so.  For dd_so_symlink it is
the patched .so plus its versioned symlinks.  Decorators replace this field;
they do not accumulate on top of the previous stage's value.
""",
        "transitive_deps_files": """\
depset[File].  Packaging files from *all transitive dependencies*, i.e. the
union of all_files from every dependency that carries DdPackagingInfo.
Decorators forward this field unchanged from their source.
""",
    },
)

def all_packaging_files(info):
    """Returns a depset of all files to install for a DdPackagingInfo instance.

    This is the union of direct_files and transitive_deps_files.  Use this
    when you need the complete file set for a target and all its dependencies.

    Args:
        info: a DdPackagingInfo instance.

    Returns:
        depset[File]
    """
    return depset(transitive = [info.direct_files, info.transitive_deps_files])
