"""Repository rule to retrieve source packages from integrations-core."""

load("@toml.bzl", "toml")
load("//bazel/repo:release_json.bzl", "read_effective_release_json")

LINUX_X86_64 = "@@//:linux_x86_64"
LINUX_ARM64 = "@@//:linux_arm64"
MACOS_X86_64 = "@@//:macos_x86_64"
MACOS_ARM64 = "@@//:macos_arm64"
WINDOWS_X86_64 = "@@//:windows_x86_64"

PLATFORMS = [
    LINUX_X86_64,
    LINUX_ARM64,
    MACOS_X86_64,
    MACOS_ARM64,
    WINDOWS_X86_64,
]

ARM_EXCLUSIONS = ["ibm_ace", "ibm_mq"]
EXCLUSIONS = [
    "tokumx",  # py2-only, unsupported by current Agent
]

INTEGRATION_CONFIGURATION_FILENAMES = [
    "conf.yaml.example",
    "conf.yaml.default",
    "metrics.yaml",
    "auto_conf.yaml",
]

SNMP_PROFILE_DIRS = ["profiles", "default_profiles"]

INTEGRATION_CONFIGURATION_WHEEL_EXCLUDES = [
    "/datadog_checks/*/data/{}".format(filename)
    for filename in INTEGRATION_CONFIGURATION_FILENAMES
] + [
    "/datadog_checks/snmp/data/{}/**".format(dir)
    for dir in SNMP_PROFILE_DIRS
]

def _integration_source_packages_impl(rctx):
    release_info = read_effective_release_json(rctx, rctx.attr._release_info)
    commit = release_info["dependencies"]["INTEGRATIONS_CORE_VERSION"]

    # https://docs.github.com/en/repositories/working-with-files/using-files/downloading-source-code-archives#source-code-archive-urls
    # Disallowing `/` prevents retrieval of mutable references via the archive URLs
    if "/" in commit:
        fail("INTEGRATIONS_CORE_VERSION must be a commit hash and not a mutable reference (got {}, which includes a `/`)".format(commit))

    # Note: this requires INTEGRATIONS_CORE_VERSION to be a full commit hash.
    # This relies on omnibus-wrapping code for resolution in cases where the original INTEGRATIONS_CORE_VERSION
    # is set to a mutable reference.
    rctx.download_and_extract(
        url = "{base_url}/archive/{commit}.tar.gz".format(
            base_url = rctx.attr.base_url,
            commit = commit,
        ),
        stripPrefix = "integrations-core-{}".format(commit),
    )

    base_packages = [
        struct(name = name, configuration_targets = [])
        for name in ["datadog_checks_base", "datadog_checks_downloader"]
    ]
    integrations = collect_integrations(rctx, arm_incompatible_integrations = ARM_EXCLUSIONS)

    # Individual packages that need to be built get their own BUILD file for building them as wheels.
    for package in base_packages + integrations:
        rctx.file(
            "{}/BUILD.bazel".format(package.name),
            _build_file_for_package(package),
        )

    # Top-level BUILD file contains references to groups of wheels that are meaningful as a unit,
    # as well as a group for packaging configuration.
    rctx.file(
        "BUILD.bazel",
        _root_build_file(
            base_packages = base_packages,
            commit = commit,
            integrations = integrations,
        ),
    )

    return rctx.repo_metadata(reproducible = True)

def collect_integrations(rctx, *, arm_incompatible_integrations = []):
    """Collects toplevel integration packages and their metadata.

    Args:
      rctx: The repository_ctx to use.
      arm_incompatible_integrations: Integrations that can't be shipped for ARM systems.

    Returns:
      A list of integration package structs, sorted by name, each containing `name`,
      `configuration_targets` and `platforms` fields.
    """
    manifest_platform_overrides = _load_manifest_platform_overrides(rctx)

    integrations = []

    for entry in rctx.path(".").readdir():
        integration_name = entry.basename
        if not entry.is_dir or integration_name in EXCLUSIONS:
            continue

        # Skip folders that are not Python packages.
        if not entry.get_child("pyproject.toml").exists:
            continue

        platforms = _bazel_platforms_for_integration(
            integration_name,
            _supported_platforms(rctx, entry, manifest_platform_overrides),
            arm_incompatible_integrations = arm_incompatible_integrations,
        )
        if not platforms:
            continue

        integrations.append(struct(
            name = integration_name,
            configuration_targets = _configuration_targets(rctx, integration_name),
            platforms = platforms,
        ))

    return integrations

def _bazel_platforms_for_integration(integration_name, supported_platforms, *, arm_incompatible_integrations = []):
    platforms = []

    if "linux" in supported_platforms:
        platforms.append(LINUX_X86_64)
        if integration_name not in arm_incompatible_integrations:
            platforms.append(LINUX_ARM64)
    if "mac_os" in supported_platforms:
        platforms.append(MACOS_X86_64)
        if integration_name not in arm_incompatible_integrations:
            platforms.append(MACOS_ARM64)
    if "windows" in supported_platforms:
        platforms.append(WINDOWS_X86_64)

    return platforms

def _load_manifest_platform_overrides(rctx):
    """Reads .ddev/config.toml manifest platform overrides for integrations without manifest.json."""
    config_path = rctx.path(".ddev/config.toml")
    if not config_path.exists:
        return {}

    config = toml.decode(rctx.read(config_path))
    if config == None:
        fail("Failed to parse {} as TOML".format(config_path))

    return config.get("overrides", {}).get("manifest", {}).get("platforms", {})

def _supported_platforms(rctx, entry, manifest_platform_overrides):
    """Returns the supported platforms for a package, using manifest.json or .ddev/config.toml overrides."""
    integration_name = entry.basename
    manifest_path = entry.get_child("manifest.json")
    if not manifest_path.exists:
        return manifest_platform_overrides.get(integration_name, [])

    manifest = json.decode(rctx.read(manifest_path))
    classifier_tags = manifest["tile"]["classifier_tags"]

    supported_platforms = []
    if "Supported OS::Linux" in classifier_tags:
        supported_platforms.append("linux")
    if "Supported OS::macOS" in classifier_tags:
        supported_platforms.append("mac_os")
    if "Supported OS::Windows" in classifier_tags:
        supported_platforms.append("windows")

    return supported_platforms

def _configuration_targets(rctx, pkg):
    """Look for config and snmp profile files under an integration and collect relevant information for target generation."""
    integration_path = rctx.path(pkg)
    name = integration_path.basename
    data_path = integration_path.get_child("datadog_checks").get_child(name).get_child("data")

    if not data_path.is_dir:
        return []

    targets = []
    configuration_files = []
    for filename in INTEGRATION_CONFIGURATION_FILENAMES:
        configuration_file = data_path.get_child(filename)
        if configuration_file.exists:
            configuration_files.append("datadog_checks/{}/data/{}".format(name, filename))

    if configuration_files:
        targets.append(struct(
            name = "{}_configuration_files".format(name),
            srcs = configuration_files,
            prefix = "{}.d".format(name),
            strip_prefix = "datadog_checks/{}/data".format(name),
        ))

    if name == "snmp":
        for profiles_dir in SNMP_PROFILE_DIRS:
            profiles_path = data_path.get_child(profiles_dir)
            if profiles_path.is_dir:
                targets.append(struct(
                    name = "snmp_{}_files".format(profiles_dir),
                    srcs = sorted([
                        "datadog_checks/snmp/data/{}/{}".format(profiles_dir, entry.basename)
                        for entry in profiles_path.readdir()
                    ]),
                    prefix = "snmp.d/{}".format(profiles_dir),
                    strip_prefix = "datadog_checks/snmp/data/{}".format(profiles_dir),
                ))

    return targets

def _build_file_for_package(package):
    return """
load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_filegroup", "pkg_files", "strip_prefix")
load("@@//deps/agent_integrations:defs.bzl", "pyproject_wheel")

package(default_visibility = ["//visibility:public"])

pyproject_wheel(
    name = "wheel",
    srcs = glob(["**"]),
    pyproject = "pyproject.toml",
    exclude = {wheel_excludes},
)
{configuration_targets}
""".format(
        configuration_targets = _render_configuration_targets(package.configuration_targets),
        wheel_excludes = repr(INTEGRATION_CONFIGURATION_WHEEL_EXCLUDES),
    )

def _render_configuration_targets(configuration_targets):
    if not configuration_targets:
        return ""

    rendered_targets = []
    for target in configuration_targets:
        rendered_targets.append(_render_configuration_target(target))

    rendered_targets.append("""
pkg_filegroup(
    name = "configuration_files",
    srcs = {srcs},
)
""".format(
        srcs = repr([":{}".format(target.name) for target in configuration_targets]),
    ))
    return "".join(rendered_targets)

def _render_configuration_target(target):
    return """
pkg_files(
    name = {name},
    srcs = {srcs},
    attributes = pkg_attributes(mode = "0644"),
    prefix = {prefix},
    strip_prefix = strip_prefix.from_pkg({strip_prefix}),
)
""".format(
        name = repr(target.name),
        prefix = repr(target.prefix),
        srcs = repr(target.srcs),
        strip_prefix = repr(target.strip_prefix),
    )

def _root_build_file(base_packages, integrations, commit):
    base_wheel_srcs = ["//{}:wheel".format(package.name) for package in base_packages]
    integrations_select = _render_platform_select(integrations, "//{}:wheel")
    integrations_configuration_select = _render_platform_select(
        [
            integration
            for integration in integrations
            if integration.configuration_targets
        ],
        "//{}:configuration_files",
    )

    return """
load("@package_metadata//rules:package_metadata.bzl", "package_metadata")
load("@rules_license//rules:license.bzl", "license")
load("@rules_pkg//pkg:mappings.bzl", "pkg_filegroup")

package(default_visibility = ["//visibility:public"])

package_metadata(
    name = "package_metadata",
    attributes = [":license"],
    purl = "pkg:github/DataDog/integrations-core@{commit}",
)

license(
    name = "license",
    license_kinds = ["@rules_license//licenses/spdx:BSD-3-Clause"],
    license_text = "LICENSE",
)

exports_files(["requirements-agent-release.txt"])

filegroup(
    name = "all_wheels",
    srcs = ["base_wheels", "integrations_wheels"],
    package_metadata = [":package_metadata"],
)

filegroup(
    name = "base_wheels",
    srcs = {base_wheel_srcs},
)

filegroup(
    name = "integrations_wheels",
    srcs = {integrations_select},
)

pkg_filegroup(
    name = "integrations_configuration_files",
    srcs = {integrations_configuration_select},
)
""".format(
        base_wheel_srcs = repr(base_wheel_srcs),
        commit = commit,
        integrations_configuration_select = integrations_configuration_select,
        integrations_select = integrations_select,
    )

def _render_platform_select(integrations, label_format):
    return "select({})".format(repr({
        platform: [
            label_format.format(integration.name)
            for integration in integrations
            if platform in integration.platforms
        ]
        for platform in PLATFORMS
    }))

integration_source_packages = repository_rule(
    implementation = _integration_source_packages_impl,
    attrs = {
        "base_url": attr.string(
            default = "https://github.com/DataDog/integrations-core",
            doc = "Base URL of the repository",
        ),
        "_release_info": attr.label(default = "//:release.json", allow_single_file = True),
    },
    doc = "Retrieves integrations-core source packages used to build Agent integration wheels.",
)
