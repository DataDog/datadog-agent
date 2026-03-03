load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@rules_pkg//pkg:providers.bzl", "PackageFilesInfo")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:util.bzl", "util")
load(":dd_agent_expand_template.bzl", "dd_agent_expand_template")

def template_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _dd_expand_basics_test,
            _dd_expand_package_info_test,
            _dd_expand_flags_in_vars_test,
        ],
    )

def _dd_expand_basics_test(name):
    util.helper_target(
        dd_agent_expand_template,
        name = name + "_basics_test",
        template = "ignored",
        substitutions = {"@BIZ@": "BAZ"},
        out = "basics_test.out",
    )
    analysis_test(
        name = name,
        impl = _basics_test_impl,
        target = name + "_basics_test",
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

def _basics_test_impl(env, target):
    subject = env.expect.that_target(target)
    expect_output_path = target.label.package + "/basics_test.out"

    # Writing the correct output file
    env.expect.that_target(target).default_outputs().contains(expect_output_path)

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

def _dd_expand_package_info_test(name):
    util.helper_target(
        dd_agent_expand_template,
        name = name + "_package_info_test",
        template = "ignored",
        out = "package_info_test.out",
        attributes = json.encode({"owner": "toot"}),
        prefix = "put/them/here",
    )
    analysis_test(
        name = name,
        impl = _package_info_test_impl,
        target = name + "_package_info_test",
    )

def _package_info_test_impl(env, target):
    # Did we put the right things in the rules_pkg provider.
    pfi = target[PackageFilesInfo]
    env.expect.that_dict(pfi.attributes).contains_exactly({"owner": "toot"})
    env.expect.that_dict(pfi.dest_src_map).keys().contains("put/them/here/package_info_test.out")

def _dd_expand_flags_in_vars_test(name):
    util.helper_target(
        dd_agent_expand_template,
        name = name + "_flags_in_vars_test",
        template = "ignored",
        substitutions = {
            "@CAP_INSTALL_DIR@": "{install_dir}",
            "@X@": "{x}",
            "@NO_CHAINING@": "@CAP_INSTALL_DIR@",
        },
        out = "flags_in_vars_test.out",
    )
    analysis_test(
        name = name,
        impl = _flags_in_vars_test_impl,
        target = name + "_flags_in_vars_test",
        attrs = {
            "_install_dir_actual": attr.label(default = "@@//:install_dir"),
        },
    )

def _flags_in_vars_test_impl(env, target):
    subject = env.expect.that_target(target)

    # Get the value of install_dir from the current flag setting
    install_dir_actual = env.ctx.attr._install_dir_actual[BuildSettingInfo].value
    biz_val = subject.actual.actions[0].substitutions.get("@CAP_INSTALL_DIR@")
    env.expect.that_str(biz_val).equals(install_dir_actual)

    no_chaining_value = subject.actual.actions[0].substitutions.get("@NO_CHAINING@")
    env.expect.that_str(no_chaining_value).equals("@CAP_INSTALL_DIR@")

    # Since there is no flag for "x", @X@ should pass through unchanged
    value = subject.actual.actions[0].substitutions.get("@X@")
    env.expect.that_str(value).equals("{x}")
