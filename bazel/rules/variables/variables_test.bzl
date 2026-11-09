"""Tests for variables.bzl - shared build-time variables rule."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:util.bzl", "util")
load(":variables.bzl", "DdBuildTimeInfo", "variables")

def _test_provider_keys(name):
    util.helper_target(
        variables,
        name = name + "_subject",
    )
    analysis_test(
        name = name,
        impl = _test_provider_keys_impl,
        target = name + "_subject",
    )

def _test_provider_keys_impl(env, target):
    values = target[DdBuildTimeInfo].values
    env.expect.that_collection(values.keys()).contains_at_least([
        "install_dir",
        "output_config_dir",
        "etc_dir",
        "build_version",
        "base_branch",
        "milestone",
        "agent_version",
        "agent_version_url_safe",
        "agent_payload_version",
        "agent_package_version",
        "commit",
        "full_commit",
    ])

def _test_versioned_install_dir(name):
    util.helper_target(
        variables,
        name = name + "_subject",
    )
    analysis_test(
        name = name,
        impl = _test_versioned_install_dir_impl,
        target = name + "_subject",
        config_settings = {
            str(Label("//:install_dir")): "/opt/datadog-packages/datadog-agent/7.99.0-1",
            "//command_line_option:platforms": str(Label("//bazel/platforms:linux_x86_64")),
        },
    )

def _test_versioned_install_dir_impl(env, target):
    env.expect.that_str(target[DdBuildTimeInfo].values["agent_package_version"]).equals("7.99.0-1")
    env.expect.that_str(target[platform_common.TemplateVariableInfo].variables["AGENT_PACKAGE_VERSION"]).equals("7.99.0-1")

def _test_windows_package_version(name):
    util.helper_target(
        variables,
        name = name + "_subject",
    )
    analysis_test(
        name = name,
        impl = _test_windows_package_version_impl,
        target = name + "_subject",
        config_settings = {
            "//command_line_option:platforms": str(Label("//bazel/platforms:windows_x86_64")),
        },
    )

def _test_windows_package_version_impl(env, target):
    values = target[DdBuildTimeInfo].values
    env.expect.that_str(values["agent_package_version"]).equals(values["agent_version_url_safe"] + "-1")

def variables_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_provider_keys,
            _test_versioned_install_dir,
            _test_windows_package_version,
        ],
    )
