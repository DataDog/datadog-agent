"""Tests for integrations-core source package collection."""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")
load(":source_packages.bzl", "classify_integrations")

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

    def _get_child(child):
        return _make_fake_path(files, parts + [child])

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

def _sorted_classification(classification):
    return {
        platform: sorted(integrations)
        for platform, integrations in classification.items()
    }

def _classify_integrations_uses_manifests_and_overrides_impl(ctx):
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

    integrations = classify_integrations(
        rctx,
        arm_incompatible_integrations = ["ibm_mq"],
    )

    asserts.equals(
        env,
        {
            LINUX_X86_64: [
                "ibm_mq",
                "manifest_linux_macos",
                "override_all",
                "override_linux",
            ],
            LINUX_ARM64: [
                "manifest_linux_macos",
                "override_all",
                "override_linux",
            ],
            MACOS_X86_64: [
                "ibm_mq",
                "manifest_linux_macos",
                "override_all",
            ],
            MACOS_ARM64: [
                "manifest_linux_macos",
                "override_all",
            ],
            WINDOWS_X86_64: [
                "ibm_mq",
                "override_all",
            ],
        },
        _sorted_classification(integrations),
    )

    return unittest.end(env)

_classify_integrations_uses_manifests_and_overrides_test = unittest.make(
    _classify_integrations_uses_manifests_and_overrides_impl,
)

def source_packages_test_suite(name):
    unittest.suite(
        name,
        _classify_integrations_uses_manifests_and_overrides_test,
    )
