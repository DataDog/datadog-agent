"""Tests for pkg_mac_pkg."""

load("@rules_pkg//pkg:mappings.bzl", "pkg_files")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_testing//lib:util.bzl", "util")
load(":pkg_mac_pkg.bzl", "pkg_mac_pkg")

def _pkgbuild_action(env, target):
    return env.expect.that_target(target).action_named("MacPkgBuild")

# ── Test cases ───────────────────────────────────────────────────────────────

# Test 1: identifier, version, and install_location flow through to the
# MacPkgBuild action's argv unchanged.
def _test_identifier_version_install_location(name):
    util.helper_target(
        pkg_files,
        name = name + "_files",
        srcs = ["testdata/payload.txt"],
        prefix = "bin",
    )
    util.helper_target(
        pkg_mac_pkg,
        name = name + "_subject",
        srcs = [":" + name + "_files"],
        identifier = "com.datadoghq.test",
        install_location = "/opt/datadog-test",
        version = "9.8.7",
    )
    analysis_test(
        name = name,
        impl = _test_identifier_version_install_location_impl,
        target = name + "_subject",
    )

def _test_identifier_version_install_location_impl(env, target):
    _pkgbuild_action(env, target).contains_flag_values([
        ("--identifier", "com.datadoghq.test"),
        ("--version", "9.8.7"),
        ("--install-location", "/opt/datadog-test"),
    ])

# Test 2: when preinstall/postinstall/signing_identity are set, their flags
# and file paths appear in the MacPkgBuild action.
def _test_preinstall_postinstall_signing_identity_present(name):
    util.helper_target(
        pkg_files,
        name = name + "_files",
        srcs = ["testdata/payload.txt"],
        prefix = "bin",
    )
    util.helper_target(
        pkg_mac_pkg,
        name = name + "_subject",
        srcs = [":" + name + "_files"],
        identifier = "com.datadoghq.test",
        postinstall = "testdata/postinstall.sh",
        preinstall = "testdata/preinstall.sh",
        signing_identity = "Developer ID Installer: Datadog, Inc.",
        version = "1.0.0",
    )
    analysis_test(
        name = name,
        impl = _test_preinstall_postinstall_signing_identity_present_impl,
        target = name + "_subject",
    )

def _test_preinstall_postinstall_signing_identity_present_impl(env, target):
    action = _pkgbuild_action(env, target)
    action.has_flags_specified(["--preinstall", "--postinstall", "--signing-identity"])
    action.contains_flag_values([
        ("--signing-identity", "Developer ID Installer: Datadog, Inc."),
    ])
    action.argv().contains_predicate(matching.str_endswith("testdata/preinstall.sh"))
    action.argv().contains_predicate(matching.str_endswith("testdata/postinstall.sh"))

# Test 3: when preinstall/postinstall/signing_identity are left unset (the
# default), the MacPkgBuild action omits their flags entirely -- pkg_mac_pkg
# must not pass empty placeholders for them.
def _test_preinstall_postinstall_signing_identity_absent_by_default(name):
    util.helper_target(
        pkg_files,
        name = name + "_files",
        srcs = ["testdata/payload.txt"],
        prefix = "bin",
    )
    util.helper_target(
        pkg_mac_pkg,
        name = name + "_subject",
        srcs = [":" + name + "_files"],
        identifier = "com.datadoghq.test",
        version = "1.0.0",
    )
    analysis_test(
        name = name,
        impl = _test_preinstall_postinstall_signing_identity_absent_by_default_impl,
        target = name + "_subject",
    )

def _test_preinstall_postinstall_signing_identity_absent_by_default_impl(env, target):
    action = _pkgbuild_action(env, target)
    action.argv().not_contains_predicate(matching.equals_wrapper("--preinstall"))
    action.argv().not_contains_predicate(matching.equals_wrapper("--postinstall"))
    action.argv().not_contains_predicate(matching.equals_wrapper("--signing-identity"))

# ── Suite ────────────────────────────────────────────────────────────────────

def pkg_mac_pkg_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_identifier_version_install_location,
            _test_preinstall_postinstall_signing_identity_present,
            _test_preinstall_postinstall_signing_identity_absent_by_default,
        ],
    )
