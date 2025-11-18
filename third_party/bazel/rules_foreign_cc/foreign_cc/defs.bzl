"""Public entry point to all Foreign CC rules and supported APIs."""

load(":boost_build.bzl", _boost_build = "boost_build")
load(":cmake.bzl", _cmake = "cmake", _cmake_variant = "cmake_variant")
load(":configure.bzl", _configure_make = "configure_make", _configure_make_variant = "configure_make_variant")
load(":make.bzl", _make = "make", _make_variant = "make_variant")
load(":meson.bzl", _meson = "meson", _meson_with_requirements = "meson_with_requirements")
load(":ninja.bzl", _ninja = "ninja")
load(":utils.bzl", _runnable_binary = "runnable_binary")

boost_build = _boost_build
cmake = _cmake
cmake_variant = _cmake_variant
configure_make = _configure_make
configure_make_variant = _configure_make_variant
make_variant = _make_variant
make = _make
meson = _meson
ninja = _ninja
meson_with_requirements = _meson_with_requirements
runnable_binary = _runnable_binary
