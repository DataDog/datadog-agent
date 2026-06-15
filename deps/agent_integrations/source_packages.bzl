"""Repository rule to retrieve source packages from integrations-core."""

load("//bazel/repo:release_json.bzl", "read_effective_release_json")

def _integration_source_packages_impl(rctx):
    release_info = read_effective_release_json(rctx, rctx.attr._release_info)
    commit = release_info["dependencies"]["INTEGRATIONS_CORE_VERSION"]

    rctx.download_and_extract(
        url = "{base_url}/archive/{commit}.tar.gz".format(
            base_url = rctx.attr.base_url,
            commit = commit,
        ),
        stripPrefix = "integrations-core-{}".format(commit),
    )

    rctx.file(
        "BUILD.bazel",
        """
package(default_visibility = ["//visibility:public"])

filegroup(
    name = "base_wheels",
    srcs = [
        "//datadog_checks_base:wheel",
        "//datadog_checks_downloader:wheel",
    ],
)
""",
    )

    for package in ["datadog_checks_base", "datadog_checks_downloader"]:
        rctx.file(
            "{}/BUILD.bazel".format(package),
            """
load("@//deps/agent_integrations:defs.bzl", "pyproject_wheel")

package(default_visibility = ["//visibility:public"])

pyproject_wheel(
    name = "wheel",
    srcs = glob(["**"]),
    pyproject = "pyproject.toml",
)
""",
        )

    return rctx.repo_metadata(reproducible = True)

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
