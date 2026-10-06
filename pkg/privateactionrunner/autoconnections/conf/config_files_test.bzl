"""Checks that PAR packages own templates, not customer script configs."""

load("@rules_pkg//pkg:providers.bzl", "PackageFilesInfo")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test")

def _linux_template_impl(env, target):
    files = target[PackageFilesInfo]
    env.expect.that_dict(files.dest_src_map).keys().contains_exactly([
        "etc/datadog-agent/private-action-runner/script-config.yaml.example",
    ])
    env.expect.that_str(files.attributes["mode"]).equals("0400")

def _windows_template_impl(env, target):
    files = target[PackageFilesInfo]
    env.expect.that_dict(files.dest_src_map).keys().contains_exactly([
        "etc/datadog-agent/private-action-runner/powershell-script-config.yaml.example",
    ])
    env.expect.that_str(files.attributes["mode"]).equals("0400")

def config_files_test_suite(name):
    """Tests both platform mappings without compiling Agent binaries."""
    analysis_test(
        name = name + "_linux",
        impl = _linux_template_impl,
        target = ":all_files",
        config_settings = {"//command_line_option:platforms": [Label("//bazel/platforms:linux_x86_64")]},
    )
    analysis_test(
        name = name + "_windows",
        impl = _windows_template_impl,
        target = ":all_files",
        config_settings = {"//command_line_option:platforms": [Label("//bazel/platforms:windows_x86_64")]},
    )
    native.test_suite(
        name = name,
        tests = [
            name + "_linux",
            name + "_windows",
        ],
    )
