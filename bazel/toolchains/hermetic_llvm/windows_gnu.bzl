"""Builds targets for Windows x86_64 with the hermetic-llvm MinGW toolchain."""

load("@rules_cc//cc:cc_library.bzl", "cc_library")
load("@with_cfg.bzl", "with_cfg")

def _windows_gnu(rule):
    return with_cfg(
        rule,
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

windows_gnu_filegroup, _windows_gnu_filegroup_internal = _windows_gnu(native.filegroup)

# Forwards the CcInfo of its deps to windows_x86_64_msvc consumers. This only
# works for DLLs: both ABIs share the UCRT, and lld writes MSVC import libraries.
windows_gnu_cc_library, _windows_gnu_cc_library_internal = _windows_gnu(cc_library)
