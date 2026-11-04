load("@bazel_skylib//rules:native_binary.bzl", "native_binary")
load("@with_cfg.bzl", "with_cfg")

msgo_binary, _ = with_cfg(native_binary).set(Label("@rules_go//go/toolchain:sdk_name"), "msgo").build()
