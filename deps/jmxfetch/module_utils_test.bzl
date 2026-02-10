"""Tests for jmxfetch module_utils version parsing."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:util.bzl", "util")
load(":test_helpers.bzl", "VersionParseInfo", "version_parse_test_rule")

def jmxfetch_module_utils_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _parse_release_version_test,
            _parse_snapshot_version_test,
        ],
    )

def _parse_release_version_test(name):
    util.helper_target(
        version_parse_test_rule,
        name = name + "_release_test_target",
        version = "0.51.0",
        out = "release_test.txt",
    )
    analysis_test(
        name = name,
        impl = _release_version_test_impl,
        target = name + "_release_test_target",
    )

def _release_version_test_impl(env, target):
    # buildifier: disable=unused-variable
    subject = env.expect.that_target(target)

    # Get the parsing result from the provider
    parse_info = target[VersionParseInfo]

    # Verify the release version was correctly parsed
    env.expect.that_str(parse_info.version).equals("0.51.0")
    env.expect.that_bool(parse_info.is_snapshot).equals(False)
    env.expect.that_str(parse_info.url).equals(
        "https://repo1.maven.org/maven2/com/datadoghq/jmxfetch/0.51.0/jmxfetch-0.51.0-jar-with-dependencies.jar",
    )

def _parse_snapshot_version_test(name):
    util.helper_target(
        version_parse_test_rule,
        name = name + "_snapshot_test_target",
        version = "0.48.0-20230706.234900",
        out = "snapshot_test.txt",
    )
    analysis_test(
        name = name,
        impl = _snapshot_version_test_impl,
        target = name + "_snapshot_test_target",
    )

def _snapshot_version_test_impl(env, target):
    # buildifier: disable=unused-variable
    subject = env.expect.that_target(target)

    # Get the parsing result from the provider
    parse_info = target[VersionParseInfo]

    # Verify the snapshot version was correctly parsed
    env.expect.that_str(parse_info.version).equals("0.48.0-20230706.234900")
    env.expect.that_bool(parse_info.is_snapshot).equals(True)
    env.expect.that_str(parse_info.url).equals(
        "https://central.sonatype.com/repository/maven-snapshots/com/datadoghq/jmxfetch/0.48.0-20230706/jmxfetch-0.48.0-234900-jar-with-dependencies.jar",
    )
