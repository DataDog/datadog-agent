"""A module exporting symbols for Stardoc generation."""

load(
    "@rules_foreign_cc//foreign_cc:defs.bzl",
    _boost_build = "boost_build",
    _cmake = "cmake",
    _cmake_variant = "cmake_variant",
    _configure_make = "configure_make",
    _configure_make_variant = "configure_make_variant",
    _make = "make",
    _make_variant = "make_variant",
    _meson = "meson",
    _meson_with_requirements = "meson_with_requirements",
    _ninja = "ninja",
)
load(
    "@rules_foreign_cc//foreign_cc:providers.bzl",
    _ForeignCcArtifactInfo = "ForeignCcArtifactInfo",
    _ForeignCcDepsInfo = "ForeignCcDepsInfo",
)
load("@rules_foreign_cc//foreign_cc:repositories.bzl", _rules_foreign_cc_dependencies = "rules_foreign_cc_dependencies")
load("@rules_foreign_cc//foreign_cc/built_tools:cmake_build.bzl", _cmake_tool = "cmake_tool")
load("@rules_foreign_cc//foreign_cc/built_tools:make_build.bzl", _make_tool = "make_tool")
load("@rules_foreign_cc//foreign_cc/built_tools:ninja_build.bzl", _ninja_tool = "ninja_tool")
load(
    "@rules_foreign_cc//toolchains/native_tools:native_tools_toolchain.bzl",
    _ToolInfo = "ToolInfo",
    _native_tool_toolchain = "native_tool_toolchain",
)

# Rules Foreign CC symbols
boost_build = _boost_build
cmake = _cmake
cmake_tool = _cmake_tool
cmake_variant = _cmake_variant
configure_make = _configure_make
configure_make_variant = _configure_make_variant
make = _make
make_tool = _make_tool
make_variant = _make_variant
meson = _meson
meson_with_requirements = _meson_with_requirements
native_tool_toolchain = _native_tool_toolchain
ninja = _ninja
ninja_tool = _ninja_tool
rules_foreign_cc_dependencies = _rules_foreign_cc_dependencies

ForeignCcArtifactInfo = _ForeignCcArtifactInfo
ForeignCcDepsInfo = _ForeignCcDepsInfo
ToolInfo = _ToolInfo
