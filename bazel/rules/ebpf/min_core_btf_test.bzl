"""Analysis test for the min_core_btf rule.

Validates the action graph (mnemonic, declared inputs, command line) without
executing bpftool, so it is hermetic and free of any kernel-BTF runtime
assumptions. This is the cache-correctness gate: it pins down exactly which
inputs feed the action's cache key.
"""

load("@bazel_skylib//lib:unittest.bzl", "analysistest", "asserts")
load(":min_core_btf.bzl", "min_core_btf")

def _action_shape_test_impl(ctx):
    env = analysistest.begin(ctx)
    tut = analysistest.target_under_test(env)

    actions = analysistest.target_actions(env)
    min_actions = [a for a in actions if a.mnemonic == "MinCoreBtf"]
    asserts.equals(env, 1, len(min_actions), "expected exactly one MinCoreBtf action")

    action = min_actions[0]
    input_names = [f.basename for f in action.inputs.to_list()]

    # The full BTF and every CO-RE program object must be declared inputs (they
    # are what the action's cache key is content-addressed on).
    asserts.true(
        env,
        "rc-btf-test.btf" in input_names,
        "full BTF must be a declared input, got: {}".format(input_names),
    )
    asserts.true(
        env,
        "btf_test.o" in input_names,
        "CO-RE program object must be a declared input, got: {}".format(input_names),
    )

    argv = action.argv
    asserts.true(env, "gen" in argv, "argv must invoke `gen`")
    asserts.true(env, "min_core_btf" in argv, "argv must invoke `min_core_btf`")

    # The declared output is the minimized BTF.
    outputs = [f.basename for f in tut[DefaultInfo].files.to_list()]
    asserts.equals(env, ["min_btf_demo.btf"], outputs)

    return analysistest.end(env)

_action_shape_test = analysistest.make(_action_shape_test_impl)

def min_core_btf_test_suite(name):
    """Create the min_core_btf demo target plus its analysis test.

    Args:
        name: name of the generated test_suite.
    """
    min_core_btf(
        name = "min_btf_demo",
        btf = "//pkg/ebpf:testdata/rc-btf-test.btf",
        programs = ["//cmd/system-probe/subcommands/ebpf/testdata:btf_test"],
        target_compatible_with = ["@platforms//os:linux"],
        tags = ["manual"],
        visibility = ["//visibility:private"],
    )

    _action_shape_test(
        name = "min_btf_demo_action_shape_test",
        target_under_test = ":min_btf_demo",
        target_compatible_with = ["@platforms//os:linux"],
    )

    native.test_suite(
        name = name,
        tests = [":min_btf_demo_action_shape_test"],
    )
