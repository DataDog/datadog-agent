"""Repository rule to download the Agent Data Plane binary from binaries.ddbuild.io.

NOTE: binaries.ddbuild.io requires Vault / ddtool authentication. In CI the
auth token is provided automatically. For local development without Vault
credentials set AGENT_DATA_PLANE_LOCAL_BINARY to the path of a pre-built
binary and pass --repo_env=AGENT_DATA_PLANE_LOCAL_BINARY=<path> on the
bazel command line (plain `export` is stripped by --experimental_strict_repo_env).

Example:
    bzl build ... --repo_env=AGENT_DATA_PLANE_LOCAL_BINARY=/path/to/agent-data-plane-arm64
"""

NAME = "agent_data_plane"

# URL template: https://binaries.ddbuild.io/saluki/agent-data-plane-{version}-linux-{arch}.tar.gz
# FIPS variant:  https://binaries.ddbuild.io/saluki/agent-data-plane-fips-{version}-linux-{arch}.tar.gz
_URL_TEMPLATE = "{base_url}/agent-data-plane-{version}-linux-{arch}.tar.gz"
_FIPS_URL_TEMPLATE = "{base_url}/agent-data-plane-fips-{version}-linux-{arch}.tar.gz"

# Architectures to download.  The generated BUILD.bazel uses select() on
# @platforms//cpu to pick the right binary at analysis time; we therefore need
# both arches present in the repo regardless of the host arch.
_ARCHES = {
    "amd64": "x86_64",  # tarball arch name -> @platforms//cpu constraint name
    "aarch64": "arm64",  # tarball arch name -> @platforms//cpu constraint name
}

def _agent_data_plane_impl(rctx):
    local_binary = rctx.os.environ.get("AGENT_DATA_PLANE_LOCAL_BINARY", "")

    if local_binary:
        # Local override: symlink the supplied binary as both arches so the select()
        # below always resolves.  This is only intended for local development.
        rctx.symlink(local_binary, "bin/agent-data-plane-amd64")
        rctx.symlink(local_binary, "bin/agent-data-plane-aarch64")
        rctx.symlink(local_binary, "bin/agent-data-plane-fips-amd64")
        rctx.symlink(local_binary, "bin/agent-data-plane-fips-aarch64")

        # Create stub license files so that the filegroup targets below resolve at
        # analysis time.  Content is intentionally empty — local dev builds are not
        # packaged for distribution.
        rctx.execute(["mkdir", "-p", "licenses/LICENSES-amd64", "licenses/LICENSES-aarch64"])
        rctx.file("licenses/LICENSE-3rdparty-amd64.csv", content = "")
        rctx.file("licenses/LICENSE-3rdparty-aarch64.csv", content = "")

        # Write stub THIRD-PARTY license texts so that the license_dir_files_arm64 /
        # license_dir_files_amd64 pkg_files targets (which glob licenses/LICENSES-{arch}/**)
        # produce non-empty output matching the 13 SPDX standard-license files expected at
        # LICENSES/LICENSES/ in production builds.  Stub content is minimal — these files are
        # only present so the packaging target resolves; they are never shipped.
        _stub_license_names = [
            "THIRD-PARTY-0BSD",
            "THIRD-PARTY-Apache-2.0",
            "THIRD-PARTY-BSD-2-Clause",
            "THIRD-PARTY-BSD-3-Clause",
            "THIRD-PARTY-BSL-1.0",
            "THIRD-PARTY-ISC",
            "THIRD-PARTY-LGPL-2.1-or-later",
            "THIRD-PARTY-MIT",
            "THIRD-PARTY-MPL-2.0",
            "THIRD-PARTY-OpenSSL",
            "THIRD-PARTY-Unicode-3.0",
            "THIRD-PARTY-Unlicense",
            "THIRD-PARTY-Zlib",
        ]
        for _name in _stub_license_names:
            _stub_content = "# Stub license file for local development builds. Not for distribution.\n# License: {}\n".format(_name)
            rctx.file("licenses/LICENSES-amd64/" + _name, content = _stub_content)
            rctx.file("licenses/LICENSES-aarch64/" + _name, content = _stub_content)
    else:
        version = rctx.attr.version
        base_url = rctx.attr.base_url

        for tar_arch, _cpu_constraint in _ARCHES.items():
            url = _URL_TEMPLATE.format(base_url = base_url, version = version, arch = tar_arch)
            sha256 = rctx.attr.linux_amd64_sha256 if tar_arch == "amd64" else rctx.attr.linux_arm64_sha256
            result = rctx.download_and_extract(
                url = url,
                sha256 = sha256,
                stripPrefix = "",
            )

            # Tarball layout (per omnibus/config/software/datadog-agent-data-plane.rb):
            #   opt/datadog-agent/embedded/bin/agent-data-plane  (the binary)
            #   opt/datadog/agent-data-plane/LICENSES/            (license texts)
            #   opt/datadog/agent-data-plane/LICENSE-3rdparty.csv (license CSV)
            # Rename to arch-specific names so both arches can coexist in the same repo.
            rctx.execute(["mkdir", "-p", "bin", "licenses"])
            rctx.execute([
                "mv",
                "opt/datadog-agent/embedded/bin/agent-data-plane",
                "bin/agent-data-plane-" + tar_arch,
            ])
            rctx.execute([
                "cp",
                "-r",
                "opt/datadog/agent-data-plane/LICENSES",
                "licenses/LICENSES-" + tar_arch,
            ])
            rctx.execute([
                "cp",
                "opt/datadog/agent-data-plane/LICENSE-3rdparty.csv",
                "licenses/LICENSE-3rdparty-" + tar_arch + ".csv",
            ])

        for tar_arch, _cpu_constraint in _ARCHES.items():
            url = _FIPS_URL_TEMPLATE.format(base_url = base_url, version = version, arch = tar_arch)
            sha256 = rctx.attr.fips_linux_amd64_sha256 if tar_arch == "amd64" else rctx.attr.fips_linux_arm64_sha256
            result = rctx.download_and_extract(
                url = url,
                sha256 = sha256,
                stripPrefix = "",
            )
            rctx.execute([
                "mv",
                "opt/datadog-agent/embedded/bin/agent-data-plane",
                "bin/agent-data-plane-fips-" + tar_arch,
            ])

    rctx.file("BUILD.bazel", content = """\
# Generated by agent_data_plane_configure.bzl — do not edit.
#
# NOTE: fetching these binaries requires Vault/ddtool auth against
# binaries.ddbuild.io.  Without credentials the download will fail.
# For local dev set AGENT_DATA_PLANE_LOCAL_BINARY and pass
# --repo_env=AGENT_DATA_PLANE_LOCAL_BINARY=<path>.

load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")

exports_files(glob(["bin/*", "licenses/**"], allow_empty = True))

# Non-FIPS binary, selected by target CPU arch.
alias(
    name = "bin/agent-data-plane",
    actual = select({
        "@platforms//cpu:x86_64": ":_adp_amd64",
        "@platforms//cpu:arm64": ":_adp_arm64",
    }),
    visibility = ["//visibility:public"],
)

# FIPS binary, selected by target CPU arch.
alias(
    name = "bin/agent-data-plane-fips",
    actual = select({
        "@platforms//cpu:x86_64": ":_adp_fips_amd64",
        "@platforms//cpu:arm64": ":_adp_fips_arm64",
    }),
    visibility = ["//visibility:public"],
)

filegroup(name = "_adp_amd64", srcs = ["bin/agent-data-plane-amd64"], visibility = ["//visibility:private"])
filegroup(name = "_adp_arm64", srcs = ["bin/agent-data-plane-aarch64"], visibility = ["//visibility:private"])
filegroup(name = "_adp_fips_amd64", srcs = ["bin/agent-data-plane-fips-amd64"], visibility = ["//visibility:private"])
filegroup(name = "_adp_fips_arm64", srcs = ["bin/agent-data-plane-fips-aarch64"], visibility = ["//visibility:private"])

# License CSV file, selected by target CPU arch.
# Omnibus: copy opt/datadog/agent-data-plane/LICENSE-3rdparty.csv
#          → LICENSES/LICENSE-agent-data-plane-3rdparty.csv
alias(
    name = "license_csv",
    actual = select({
        "@platforms//cpu:x86_64": ":_lic_csv_amd64",
        "@platforms//cpu:arm64": ":_lic_csv_arm64",
    }),
    visibility = ["//visibility:public"],
)

filegroup(name = "_lic_csv_amd64", srcs = ["licenses/LICENSE-3rdparty-amd64.csv"], visibility = ["//visibility:private"])
filegroup(name = "_lic_csv_arm64", srcs = ["licenses/LICENSE-3rdparty-aarch64.csv"], visibility = ["//visibility:private"])

# pkg_files that places the license CSV at LICENSES/LICENSE-agent-data-plane-3rdparty.csv.
# Consumers reference this target directly; arch selection happens via the alias above.
pkg_files(
    name = "license_csv_files",
    srcs = [":license_csv"],
    prefix = "LICENSES",
    renames = select({
        "@platforms//cpu:x86_64": {":_lic_csv_amd64": "LICENSE-agent-data-plane-3rdparty.csv"},
        "@platforms//cpu:arm64": {":_lic_csv_arm64": "LICENSE-agent-data-plane-3rdparty.csv"},
    }),
    target_compatible_with = ["@platforms//os:linux"],
    visibility = ["//visibility:public"],
)

# License texts directory.
# Omnibus: copy opt/datadog/agent-data-plane/LICENSES → LICENSES/LICENSES
# (LICENSES dir already exists in install_dir, so the copy nests as LICENSES/LICENSES/).
# Arch-specific strip_prefix is hard-coded per target since strip_prefix is a string attr
# and glob() cannot appear inside select().
pkg_files(
    name = "license_dir_files_amd64",
    srcs = glob(["licenses/LICENSES-amd64/**"], allow_empty = True),
    prefix = "LICENSES/LICENSES",
    strip_prefix = "licenses/LICENSES-amd64",
    target_compatible_with = ["@platforms//os:linux"],
    visibility = ["//visibility:public"],
)

pkg_files(
    name = "license_dir_files_arm64",
    srcs = glob(["licenses/LICENSES-aarch64/**"], allow_empty = True),
    prefix = "LICENSES/LICENSES",
    strip_prefix = "licenses/LICENSES-aarch64",
    target_compatible_with = ["@platforms//os:linux"],
    visibility = ["//visibility:public"],
)

pkg_files(
    name = "all_files",
    srcs = [":bin/agent-data-plane"],
    attributes = pkg_attributes(mode = "0755"),
    prefix = "embedded/bin",
    target_compatible_with = ["@platforms//os:linux"],
    visibility = ["//visibility:public"],
)
""")

download_agent_data_plane = repository_rule(
    implementation = _agent_data_plane_impl,
    doc = """\
Download the Agent Data Plane (saluki) binary from binaries.ddbuild.io.

Requires Vault / ddtool auth. For local development without Vault credentials,
set AGENT_DATA_PLANE_LOCAL_BINARY to a local binary path and pass
--repo_env=AGENT_DATA_PLANE_LOCAL_BINARY=<path>.
""",
    attrs = {
        "base_url": attr.string(
            default = "https://binaries.ddbuild.io/saluki",
            doc = "Base URL for the binaries server.",
        ),
        "version": attr.string(
            mandatory = True,
            doc = "Agent Data Plane version to download, e.g. '1.1.2'.",
        ),
        "linux_amd64_sha256": attr.string(
            mandatory = True,
            doc = "SHA256 checksum of the linux-amd64 tarball.",
        ),
        "linux_arm64_sha256": attr.string(
            mandatory = True,
            doc = "SHA256 checksum of the linux-aarch64 tarball.",
        ),
        "fips_linux_amd64_sha256": attr.string(
            mandatory = True,
            doc = "SHA256 checksum of the FIPS linux-amd64 tarball.",
        ),
        "fips_linux_arm64_sha256": attr.string(
            mandatory = True,
            doc = "SHA256 checksum of the FIPS linux-aarch64 tarball.",
        ),
    },
    environ = ["AGENT_DATA_PLANE_LOCAL_BINARY"],
)

_configure = tag_class(
    attrs = {
        "base_url": attr.string(default = "https://binaries.ddbuild.io/saluki"),
        "version": attr.string(mandatory = True),
        "linux_amd64_sha256": attr.string(mandatory = True),
        "linux_arm64_sha256": attr.string(mandatory = True),
        "fips_linux_amd64_sha256": attr.string(mandatory = True),
        "fips_linux_arm64_sha256": attr.string(mandatory = True),
    },
)

def _agent_data_plane_extension_impl(ctx):
    cfg = ctx.modules[0].tags.configure[0]
    download_agent_data_plane(
        name = NAME,
        base_url = cfg.base_url,
        version = cfg.version,
        linux_amd64_sha256 = cfg.linux_amd64_sha256,
        linux_arm64_sha256 = cfg.linux_arm64_sha256,
        fips_linux_amd64_sha256 = cfg.fips_linux_amd64_sha256,
        fips_linux_arm64_sha256 = cfg.fips_linux_arm64_sha256,
    )

agent_data_plane_configure = module_extension(
    implementation = _agent_data_plane_extension_impl,
    tag_classes = {"configure": _configure},
    os_dependent = True,
    arch_dependent = True,
)
