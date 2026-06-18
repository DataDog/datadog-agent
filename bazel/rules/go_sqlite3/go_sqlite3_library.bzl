"""go_library wrapper that links github.com/mattn/go-sqlite3 against //deps:sqlite3."""

load("@rules_go//go:def.bzl", _go_library = "go_library")

def go_sqlite3_library(**kwargs):
    """Links mattn/go-sqlite3 against the shared libsqlite3 from //deps:sqlite3.

    Wraps go_library so the Gazelle-generated rule for mattn/go-sqlite3
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
        Gazelle-emitted entries are preserved. clinkopts is dropped: Gazelle
        lifts mattn's `#cgo {darwin,linux,windows,...} LDFLAGS: -lsqlite3`
        into this attribute, and the per-OS variants (notably
        `-L/usr/local/opt/sqlite/lib` on darwin_amd64 and
        `-L/opt/homebrew/opt/sqlite/lib` on darwin_arm64) get the linker to
        resolve `-lsqlite3` against the macOS SDK sysroot's `libsqlite3.tbd`
        (Apple's system libsqlite3, built with SQLITE_OMIT_LOAD_EXTENSION).
        The cdep's cc_import → cc_shared_library chain provides the right
        `-L`/`-l` linker arguments for our libsqlite3 on every platform, so
        the Gazelle-lifted entries are redundant at best and broken at worst.
    """
    kwargs["cgo"] = True
    cdeps = kwargs.pop("cdeps", [])
    label = "@@//bazel/rules/go_sqlite3:libsqlite3_dynamic"
    if label not in cdeps:
        cdeps = cdeps + [label]
    kwargs.pop("clinkopts", None)
    _go_library(cdeps = cdeps, **kwargs)
