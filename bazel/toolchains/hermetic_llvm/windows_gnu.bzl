"""Builds targets for Windows x86_64 with the hermetic-llvm MinGW toolchain."""

load("@with_cfg.bzl", "with_cfg")

windows_gnu_filegroup, _windows_gnu_filegroup_internal = with_cfg(
    native.filegroup,
).set(
    "platforms",
    [Label(":windows_x86_64_gnu")],
).extend(
    "extra_toolchains",
    [
        # Not registered globally: their target_settings also match the
        # unconstrained ABI of //bazel/platforms:windows_x86_64, which would
        # give Linux-hosted builds for that platform a cc toolchain.
        str(Label("@llvm//toolchain:linux_aarch64_to_windows_x86_64")),
        str(Label("@llvm//toolchain:linux_x86_64_to_windows_x86_64")),
        str(Label(":linux_make_toolchain")),
    ],
).build()
