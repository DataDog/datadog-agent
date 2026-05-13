"""go_library wrapper that links github.com/mattn/go-sqlite3 against //deps:sqlite3."""

load("@rules_go//go:def.bzl", _go_library = "go_library")

def go_sqlite3_library(**kwargs):
    """Wraps go_library so the Gazelle-generated rule for mattn/go-sqlite3
    enables cgo and pulls in @sqlite3//:libsqlite3_dynamic as a cdep.

    Wired in via gazelle:map_kind on github.com/mattn/go-sqlite3 (see
    deps/go.MODULE.bazel). The libsqlite3 Go build tag is enabled globally
    in .bazelrc, so sqlite3_libsqlite3.go defines USE_LIBSQLITE3 and the
    bundled amalgamation in sqlite3-binding.c is a no-op TU.

    TODO(agent-build): when the agent binary is built with Bazel, its rpath
    will point into bazel-bin/_solib_local/... and won't resolve at install
    time. Apply the same packaging path used for cpython: pull
    @sqlite3//:sqlite3_pkg into the binary's packaging deps and run the
    bazel/rules/rewrite_rpath rule post-link to retarget to the install-tree
    libsqlite3.so location.

    Args:
      **kwargs: forwarded to go_library. cdeps is extended (not replaced) so
        Gazelle-emitted entries are preserved.
    """
    kwargs["cgo"] = True
    cdeps = kwargs.pop("cdeps", [])
    label = "@@//bazel/rules/go_sqlite3:libsqlite3_dynamic"
    if label not in cdeps:
        cdeps = cdeps + [label]
    _go_library(cdeps = cdeps, **kwargs)
