"""Builds targets for Windows x86_64 with the MSVC ABI (clang-cl)."""

load("@with_cfg.bzl", "with_cfg")

windows_msvc_filegroup, _windows_msvc_filegroup_internal = with_cfg(
    native.filegroup,
).set(
    "platforms",
    [Label("//bazel/platforms:windows_x86_64_msvc")],
).build()
