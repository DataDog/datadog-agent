"""Exposes the proto compiler from the registered proto toolchain."""

load("@protobuf//bazel/private:toolchain_helpers.bzl", "toolchains")

def _impl(ctx):
    protoc = ctx.toolchains[toolchains.PROTO_TOOLCHAIN].proto.proto_compiler.executable
    if not protoc.is_source:
        fail("expected a prebuilt protoc binary, got {} compiled from {}".format(protoc.path, protoc.owner))
    alias = ctx.actions.declare_file(protoc.basename)
    ctx.actions.symlink(output = alias, target_file = protoc)
    return [DefaultInfo(executable = alias)]

protoc = rule(_impl, executable = True, toolchains = [toolchains.PROTO_TOOLCHAIN])
