"""Builds an e2e suite against the Go SDK whose `testing` package reports to CI Visibility."""

_SDK_NAME = "@rules_go//go/toolchain:sdk_name"

_civisibility_sdk = transition(
    implementation = lambda *_: {_SDK_NAME: "go_civisibility_sdk"},
    inputs = [],
    outputs = [_SDK_NAME],
)

def _civisibility_test_binary_impl(ctx):
    return [DefaultInfo(files = depset([ctx.executable.test]))]

civisibility_test_binary = rule(
    implementation = _civisibility_test_binary_impl,
    doc = "Exposes the executable of `test` built with `go_civisibility_sdk`.",
    attrs = {
        "test": attr.label(
            doc = "The go_test target of the e2e suite.",
            cfg = _civisibility_sdk,
            executable = True,
            mandatory = True,
        ),
    },
)
