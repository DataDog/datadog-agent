"""MODULE wrapper to download jmxfetch JAR using variables from release.json.

This loads 'dependencies' from release.json and uses that to construct the
appropriate Maven repository URL based on the jmxfetch version type:
- Release versions (X.Y.Z) use Maven Central repo
- Snapshot versions (X.Y.Z-identifier-timestamp) use Maven Snapshots repo

Usage:
  get_jmxfetch_using_release_constants = use_repo_rule(":module_utils.bzl", "get_jmxfetch_using_release_constants")

  get_jmxfetch_using_release_constants(
      name = "jmxfetch",
  )
"""

load("@bazel_tools//tools/build_defs/repo:cache.bzl", "DEFAULT_CANONICAL_ID_ENV", "get_default_canonical_id")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "get_auth")

get_jmxfetch_using_release_constants_attrs = {
    "executable": attr.bool(
        default = False,
        doc = "If the downloaded file should be made executable.",
    ),
    "target_filename": attr.string(
        default = "jmxfetch.jar",
        doc = "Name assigned to the downloaded file.",
    ),
    "canonical_id": attr.string(),
    "_release_info": attr.label(default = "//:release.json", allow_single_file = True),
}

def parse_jmxfetch_version(version):
    """Parse jmxfetch version to determine if it's release or snapshot.

    Args:
      version: jmxfetch version

    Returns:
      dict: is_snapshot, url: full download URL
    """
    parts = version.split(".")

    # Release version has exactly 3 parts (X.Y.Z)
    if len(parts) == 3:
        # It's a release version
        url = "https://repo1.maven.org/maven2/com/datadoghq/jmxfetch/{VERSION}/jmxfetch-{VERSION}-jar-with-dependencies.jar".format(VERSION = version)
        return {
            "is_snapshot": False,
            "url": url,
        }

    # Otherwise it's a snapshot version like 0.48.0-20230706.234900
    # Parse using regex to extract components
    for i in range(len(version)):
        if version[i] == "-":
            # Found first dash
            before_dash = version[:i]
            after_dash = version[i + 1:]

            # Check if before_dash is X.Y.Z format
            before_parts = before_dash.split(".")
            if len(before_parts) == 3:
                # Found the base version, now parse the rest
                # after_dash should be like "20230706.234900"
                # We need to split this into snapshot_version and timestamp
                after_parts = after_dash.split(".")
                if len(after_parts) >= 2:
                    # snapshot_version is base + dash + date part
                    snapshot_version = before_dash + "-" + after_parts[0]

                    # timestamp is everything after the dot
                    timestamp = ".".join(after_parts[1:])

                    # Reconstruct timestamped_version as base_version-timestamp
                    timestamped_version = before_dash + "-" + timestamp

                    url = "https://central.sonatype.com/repository/maven-snapshots/com/datadoghq/jmxfetch/{SNAPSHOT_VERSION}/jmxfetch-{TIMESTAMPED_VERSION}-jar-with-dependencies.jar".format(
                        SNAPSHOT_VERSION = snapshot_version,
                        TIMESTAMPED_VERSION = timestamped_version,
                    )
                    return {
                        "is_snapshot": True,
                        "url": url,
                    }

    # Fallback: treat as snapshot and construct best-effort URL
    url = "https://central.sonatype.com/repository/maven-snapshots/com/datadoghq/jmxfetch/{VERSION}/jmxfetch-{VERSION}-jar-with-dependencies.jar".format(VERSION = version)
    return {
        "is_snapshot": True,
        "url": url,
    }

def _get_jmxfetch_using_release_constants_impl(rctx):
    """Implementation of the get_jmxfetch_using_release_constants rule."""
    release_info = json.decode(rctx.read(rctx.path(rctx.attr._release_info)))
    vars = release_info["dependencies"]

    version = vars["JMXFETCH_VERSION"]
    sha256 = vars["JMXFETCH_HASH"]

    # Parse version and get URL
    version_info = parse_jmxfetch_version(version)
    url = version_info["url"]
    is_snapshot = version_info["is_snapshot"]

    # Determine license file version: 'master' for snapshots, version for releases
    license_file_version = "master" if is_snapshot else version

    repo_root = rctx.path(".")
    forbidden_files = [
        repo_root,
        rctx.path("MODULE.bazel"),
        rctx.path("BUILD"),
    ]
    target_filename = rctx.attr.target_filename
    download_path = rctx.path("file/" + target_filename)

    if download_path in forbidden_files or not str(download_path).startswith(str(repo_root)):
        fail("'%s' cannot be used as target_filename in get_jmxfetch_using_release_constants" % rctx.attr.target_filename)

    # Download the JAR file
    rctx.download(
        [url],
        target_filename,
        sha256,
        rctx.attr.executable,
        canonical_id = rctx.attr.canonical_id or get_default_canonical_id(rctx, [url]),
        auth = get_auth(rctx, [url]),
    )

    # Download the LICENSE file
    license_url = "https://raw.githubusercontent.com/DataDog/jmxfetch/{version}/LICENSE".format(version = license_file_version)
    rctx.download(
        [license_url],
        "LICENSE",
    )

    rctx.file("MODULE.bazel", "module(name = \"{name}\")\n".format(name = rctx.name))

    # Use template to generate BUILD file
    rctx.template(
        "BUILD",
        Label("//deps/jmxfetch:jmxfetch.BUILD.bazel"),
        substitutions = {
            "{target_filename}": target_filename,
        },
    )

    return rctx.repo_metadata(reproducible = True)

get_jmxfetch_using_release_constants = repository_rule(
    implementation = _get_jmxfetch_using_release_constants_impl,
    attrs = get_jmxfetch_using_release_constants_attrs,
    environ = [DEFAULT_CANONICAL_ID_ENV],
)
