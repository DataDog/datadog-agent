"""Repository rule to retrieve source packages from integrations-core."""

load("//bazel/repo:release_json.bzl", "read_effective_release_json")

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

    packages = ["datadog_checks_base", "datadog_checks_downloader"]

    # Top-level BUILD file contains references to groups of wheels that are meaningful as a unit
    wheel_srcs = ", ".join(['"//{}:wheel"'.format(pkg) for pkg in packages])
    rctx.file(
        "BUILD.bazel",
        """
package(default_visibility = ["//visibility:public"])

filegroup(
    name = "base_wheels",
    srcs = [{}],
)
""".format(wheel_srcs),
    )

    # Individual packages that need to be built get their own BUILD file for building them as wheels
    for pkg in packages:
        rctx.file(
            "{}/BUILD.bazel".format(pkg),
            """
load("@@//deps/agent_integrations:defs.bzl", "pyproject_wheel")

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
