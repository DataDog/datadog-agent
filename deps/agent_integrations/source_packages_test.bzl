"""Tests for integrations-core source package collection."""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")
load(":source_packages.bzl", "collect_integrations", "filter_integration_requirements")

LINUX_X86_64 = "@@//:linux_x86_64"
LINUX_ARM64 = "@@//:linux_arm64"
MACOS_X86_64 = "@@//:macos_x86_64"
MACOS_ARM64 = "@@//:macos_arm64"
WINDOWS_X86_64 = "@@//:windows_x86_64"

ALL_SUPPORTED_TAGS = [
    "Supported OS::Linux",
    "Supported OS::macOS",
    "Supported OS::Windows",
]

def _manifest(classifier_tags):
    return json.encode({
        "tile": {
            "classifier_tags": classifier_tags,
        },
    })

def _lookup(files, parts):
    node = files
    for part in parts:
        if type(node) != "dict" or part not in node:
            return None
        node = node[part]
    return node

def _make_fake_path(files, parts):
    node = _lookup(files, parts)

    def _readdir():
        if type(node) != "dict":
            return []
        return [_make_fake_path(files, parts + [name]) for name in sorted(node.keys())]

    def _get_child(*relative_paths):
        return _make_fake_path(files, parts + list(relative_paths))

    return struct(
        basename = parts[-1] if parts else ".",
        exists = node != None,
        get_child = _get_child,
        is_dir = type(node) == "dict",
        parts = parts,
        readdir = _readdir,
    )

def _make_fake_rctx(files):
    def _path(path):
        if path == ".":
            return _make_fake_path(files, [])
        return _make_fake_path(files, path.split("/"))

    def _read(path):
        return _lookup(files, path.parts)

    return struct(
        path = _path,
        read = _read,
    )

def _integration_by_name(integrations):
    return {
        integration.name: integration
        for integration in integrations
    }

def _collect_integrations_uses_manifests_and_overrides_impl(ctx):
    env = unittest.begin(ctx)

    files = {
        ".ddev": {
            "config.toml": """
[overrides.manifest.platforms]
override_all = ["linux", "windows", "mac_os"]
override_linux = ["linux"]
""",
        },
        "ibm_mq": {
            "manifest.json": _manifest(ALL_SUPPORTED_TAGS),
            "pyproject.toml": "",
        },
        "manifest_linux_macos": {
            "manifest.json": _manifest([
                "Supported OS::Linux",
                "Supported OS::macOS",
            ]),
            "pyproject.toml": "",
        },
        "kubelet": {
            "datadog_checks": {
                "kubelet": {
                    "data": {
                        "conf.yaml.example": "",
                    },
                },
            },
            "manifest.json": _manifest(ALL_SUPPORTED_TAGS),
            "pyproject.toml": "",
        },
        "no_manifest_no_override": {
            "pyproject.toml": "",
        },
        "no_pyproject": {
            "manifest.json": _manifest(ALL_SUPPORTED_TAGS),
        },
        "override_all": {
            "pyproject.toml": "",
        },
        "override_linux": {
            "pyproject.toml": "",
        },
        "tokumx": {
            "manifest.json": _manifest(ALL_SUPPORTED_TAGS),
            "pyproject.toml": "",
        },
    }
    rctx = _make_fake_rctx(files)

    integrations = _integration_by_name(collect_integrations(
        rctx,
        arm_incompatible_integrations = ["ibm_mq"],
        wheel_exclusions = ["kubelet"],
    ))

    asserts.equals(env, 5, len(integrations))
    asserts.equals(env, [LINUX_X86_64, MACOS_X86_64, WINDOWS_X86_64], integrations["ibm_mq"].platforms)
    asserts.equals(
        env,
        [LINUX_X86_64, LINUX_ARM64, MACOS_X86_64, MACOS_ARM64],
        integrations["manifest_linux_macos"].platforms,
    )
    asserts.equals(
        env,
        [LINUX_X86_64, LINUX_ARM64, MACOS_X86_64, MACOS_ARM64, WINDOWS_X86_64],
        integrations["override_all"].platforms,
    )
    asserts.equals(env, [LINUX_X86_64, LINUX_ARM64], integrations["override_linux"].platforms)
    asserts.false(env, integrations["kubelet"].ship_wheel)
    asserts.equals(env, 1, len(integrations["kubelet"].configuration_targets))

    return unittest.end(env)

_collect_integrations_uses_manifests_and_overrides_test = unittest.make(
    _collect_integrations_uses_manifests_and_overrides_impl,
)

def _collect_integrations_includes_platforms_and_configuration_impl(ctx):
    env = unittest.begin(ctx)

    files = {
        "config_check": {
            "datadog_checks": {
                "config_check": {
                    "data": {
                        "conf.yaml.example": "",
                        "metrics.yaml": "",
                    },
                },
            },
            "manifest.json": _manifest([
                "Supported OS::Linux",
                "Supported OS::Windows",
            ]),
            "pyproject.toml": "",
        },
        "snmp": {
            "datadog_checks": {
                "snmp": {
                    "data": {
                        "conf.yaml.example": "",
                        "default_profiles": {
                            "generic-device.yaml": "",
                        },
                        "profiles": {
                            "custom.yaml": "",
                        },
                    },
                },
            },
            "manifest.json": _manifest(ALL_SUPPORTED_TAGS),
            "pyproject.toml": "",
        },
    }
    rctx = _make_fake_rctx(files)

    integrations = _integration_by_name(collect_integrations(rctx))

    asserts.equals(
        env,
        [LINUX_X86_64, LINUX_ARM64, WINDOWS_X86_64],
        integrations["config_check"].platforms,
    )
    asserts.equals(
        env,
        [
            struct(
                name = "config_check_configuration_files",
                prefix = "config_check.d",
                srcs = [
                    "datadog_checks/config_check/data/conf.yaml.example",
                    "datadog_checks/config_check/data/metrics.yaml",
                ],
                strip_prefix = "datadog_checks/config_check/data",
            ),
        ],
        integrations["config_check"].configuration_targets,
    )
    asserts.equals(
        env,
        [
            struct(
                name = "snmp_configuration_files",
                prefix = "snmp.d",
                srcs = ["datadog_checks/snmp/data/conf.yaml.example"],
                strip_prefix = "datadog_checks/snmp/data",
            ),
            struct(
                name = "snmp_profiles_files",
                prefix = "snmp.d/profiles",
                srcs = ["datadog_checks/snmp/data/profiles/custom.yaml"],
                strip_prefix = "datadog_checks/snmp/data/profiles",
            ),
            struct(
                name = "snmp_default_profiles_files",
                prefix = "snmp.d/default_profiles",
                srcs = ["datadog_checks/snmp/data/default_profiles/generic-device.yaml"],
                strip_prefix = "datadog_checks/snmp/data/default_profiles",
            ),
        ],
        integrations["snmp"].configuration_targets,
    )

    return unittest.end(env)

_collect_integrations_includes_platforms_and_configuration_test = unittest.make(
    _collect_integrations_includes_platforms_and_configuration_impl,
)

def _filter_integration_requirements_impl(ctx):
    env = unittest.begin(ctx)

    requirements = """# DO NOT PASS THIS TO PIP DIRECTLY
--no-index
datadog-kubelet==10.4.0
datadog-redisdb==8.6.0
"""
    expected = """# DO NOT PASS THIS TO PIP DIRECTLY
--no-index
datadog-redisdb==8.6.0
"""

    asserts.equals(env, expected, filter_integration_requirements(requirements, ["kubelet"]))

    return unittest.end(env)

_filter_integration_requirements_test = unittest.make(
    _filter_integration_requirements_impl,
)

def source_packages_test_suite(name):
    unittest.suite(
        name,
        _collect_integrations_uses_manifests_and_overrides_test,
        _collect_integrations_includes_platforms_and_configuration_test,
        _filter_integration_requirements_test,
    )
