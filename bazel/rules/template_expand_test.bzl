load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@rules_pkg//pkg:providers.bzl", "PackageFilesInfo")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:util.bzl", "util")
load(":dd_agent_expand_template.bzl", "dd_agent_expand_template")

def _test_basics(name):
    util.helper_target(
        dd_agent_expand_template,
        name = name + "_subject",
        template = "ignored",
        substitutions = {"@BIZ@": "BAZ"},
        out = "placeholder.out",
        attributes = json.encode({"owner": "toot"}),
        prefix = "put/them/here",
    )
    analysis_test(
        name = name,
        impl = _test_basics_impl,
        target = name + "_subject",
        attr_values = {
            "expect_cpu": select({
                "@platforms//cpu:arm64": "arm64",
                "//conditions:default": "x86_64",
            }),
        },
        attrs = {
            "expect_cpu": attr.string(),
            "_install_dir_actual": attr.label(default = "@@//:install_dir"),
        },
    )

def _test_basics_impl(env, target):
    subject = env.expect.that_target(target)
    expect_output_path = target.label.package + "/placeholder.out"

    # Writing the correct output file
    env.expect.that_target(target).default_outputs().contains(expect_output_path)

    # Did we put the right things in the rule_pkg provider.
    pfi = target[PackageFilesInfo]
    env.expect.that_dict(pfi.attributes).contains_exactly({"owner": "toot"})
    env.expect.that_dict(pfi.dest_src_map).keys().contains("put/them/here/placeholder.out")

    # Did we built the right substitution dictionary
    # Are all the expected keys here.
    action = subject.action_generating(expect_output_path)

    # print("ACTION", dir(action))
    # Intentionally using contains_at_least rather the exact match
    # so this is not brittle w.r.t. adding new things. The real test
    # is that {} is around the builtins and the user table can use
    # whatever we like.
    action.substitutions().keys().contains_at_least([
        "{output_config_dir}",
        "{install_dir}",
        "{etc_dir}",
        "{TARGET_CPU}",
        "{COMPILATION_MODE}",
        "@BIZ@",
    ])

    # The obvious syntax is: value = action.substitutions().get("@BIZ@")
    # But this does not work. get() also requires a 'factory' method, which
    # seems to be internal. At the very least it is underdocumented.
    # Instead, we can go to the actual enity to get the raw dict.
    biz_val = subject.actual.actions[0].substitutions.get("@BIZ@")
    env.expect.that_str(biz_val).equals("BAZ")

    # Testing CPU is too brittle for anything beyond having a value. It
    # varies by OS and architecture.
    cpu = subject.actual.actions[0].substitutions.get("{TARGET_CPU}")
    env.expect.that_str(cpu).not_equals("")

    install_dir = subject.actual.actions[0].substitutions.get("{install_dir}")
    install_dir_actual = env.ctx.attr._install_dir_actual[BuildSettingInfo].value
    env.expect.that_str(install_dir).equals(install_dir_actual)

def template_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_basics,
        ],
    )
