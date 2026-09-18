"""Repository rule mirroring the pinned Go SDK with a CI Visibility patch applied to `testing`.

Patches `src/testing/testing.go` and `src/testing/benchmark.go` to call the
same civisibility hooks Orchestrion's automatic instrumentation would weave
in (see `github.com/DataDog/dd-trace-go/v2/internal/civisibility/integrations/gotesting`'s
`orchestrion.yml`). The patch targets the pinned Go SDK version, not testify
or dd-trace-go, so it only needs revisiting on a Go version bump.

Two independent consumers:
- `go test -c -overlay=<this repo>/overlay.json ...`, driven from outside
  Bazel (`tasks/new_e2e_tests.py`), via the `overlay.json`/`go` files at this
  repo's root.
- A full mirror of the SDK's own directory tree (`bin/`, `pkg/`, `src/`,
  `ROOT`, ...), every entry except the two patched files symlinked straight
  back to the real SDK, so this repo can also serve as a real, independent
  GOROOT for a second `go_sdk`/`go_toolchain` registration (see BUILD.bazel):
  rules_go's own stdlib-compile action reads `$GOROOT` off disk directly
  (see `go/tools/builders/stdlib.go`'s `replicate()`), not off the Bazel
  `srcs` depset, so swapping `srcs` alone on a `go_sdk` pointed at the
  original SDK's `root_file` has no effect -- the replacement files must
  physically live under this repo's own root instead.

Modeled on `//bazel/rules/go_fast:repo.bzl`, which patches `cmd/go` itself
the same way (copy specific SDK files, apply a patch, build an overlay).
"""

load("@rules_python//python/private:repo_utils.bzl", "repo_utils")  # buildifier: disable=bzl-visibility

_PATCHED = [
    "src/testing/testing.go",
    "src/testing/benchmark.go",
]

def _impl(rctx):
    sdk = rctx.path(rctx.attr._go_sdk).dirname
    rctx.watch(sdk.get_child("VERSION"))  # invalidate on SDK bump, since the patch targets specific stdlib source
    replace = {}
    for path in _PATCHED:
        src = sdk.get_child(path)
        rctx.file(path, rctx.read(src))
        replace[str(src)] = str(rctx.path(path))
    rctx.patch(rctx.attr.patch)
    rctx.file("overlay.json", json.encode({"Replace": replace}))

    # `@go_work_sdk//:ROOT` itself never gets an execroot symlink unless some
    # action actually depends on it, so callers resolving it via `cquery
    # --output=files` (e.g. tasks/new_e2e_tests.py) get a dangling path.
    # Re-expose the SDK's own `go` binary from here instead, since this
    # repo's other files (like `overlay.json`) do materialize correctly.
    go_name = "go.exe" if repo_utils.get_platforms_os_name(rctx) == "windows" else "go"
    rctx.symlink(sdk.get_child("bin").get_child(go_name), go_name)

    # Full GOROOT mirror: symlink everything except the two patched files,
    # which already exist above as real (patched) file content.
    patched_by_dir = {}
    for path in _PATCHED:
        dir_path, _, name = path.rpartition("/")
        patched_by_dir.setdefault(dir_path, []).append(name)

    for name in ["bin", "pkg"]:
        rctx.symlink(sdk.get_child(name), name)
    for name in ["ROOT", "VERSION", "go.env"]:
        child = sdk.get_child(name)
        if child.exists:
            rctx.symlink(child, name)

    for entry in sdk.get_child("src").readdir():
        rel = "src/" + entry.basename
        if rel in patched_by_dir:
            for sub in entry.readdir():
                if sub.basename not in patched_by_dir[rel]:
                    rctx.symlink(sub, rel + "/" + sub.basename)

            # The patched files themselves were already written above.
        else:
            rctx.symlink(entry, rel)

    rctx.file(
        "BUILD.bazel",
        """exports_files(["overlay.json", "{go_name}", "bin/{go_name}", {patched}])

package(default_visibility = ["//visibility:public"])

filegroup(name = "root_file", srcs = ["ROOT"])

filegroup(name = "headers", srcs = glob(["pkg/include/*.h"]))

filegroup(name = "srcs", srcs = glob(["src/**/*"], allow_empty = True))

filegroup(name = "tools", srcs = glob(["pkg/tool/**", "bin/gofmt*"], allow_empty = True))

filegroup(name = "libs", srcs = glob(["pkg/*/**/*.a"], allow_empty = True, exclude = ["pkg/*/**/cmd/**"]))
""".format(
            go_name = go_name,
            patched = ", ".join(['"%s"' % p for p in _PATCHED]),
        ),
    )

go_civisibility_testing_overlay = repository_rule(
    implementation = _impl,
    attrs = {
        "_go_sdk": attr.label(default = "@go_work_sdk//:ROOT"),
        "patch": attr.label(default = "//bazel/rules/go_civisibility:testing-instrumentation.patch"),
    },
)
