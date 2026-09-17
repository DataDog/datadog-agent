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
    ])

def variables_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_provider_keys,
        ],
    )
