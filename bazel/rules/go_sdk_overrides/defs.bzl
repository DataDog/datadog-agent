"""Shared configuration for the go_sdk_overrides repository rule and write_source_files."""

load("@bazel_skylib//lib:paths.bzl", "paths")

OVERRIDES = {
    "src/html/template": Label("//pkg/template/html:BUILD.bazel"),
    "src/internal/fmtsort": Label("//pkg/template/internal/fmtsort:BUILD.bazel"),
    "src/text/template": Label("//pkg/template/text:BUILD.bazel"),
}

PATCHES = [
    # do not sort
    "//pkg/template:no-method.patch",
    "//pkg/template:imports.gopatch",
    "//pkg/template:godebug.gopatch",
    "//pkg/template:types.patch",
    "//pkg/template:godebug-stub.patch",
]

def go_sdk_overrides():
    return {
        paths.relativize(label.package, native.package_name()): "@go_sdk_overrides//:{}".format(label.package)
        for label in OVERRIDES.values()
        if label.package.startswith(native.package_name())
    }
