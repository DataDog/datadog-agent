"""Repository rule to retrieve and install integrations and their dependencies."""

load("@cpython_versions//:constants.bzl", "PYTHON_MAJOR_MINOR")
load("//bazel/repo:release_json.bzl", "read_effective_release_json")

def _detect_platform(ctx):
    """Returns a (os, arch) tuple normalized to our lockfile naming convention."""
    os_name = ctx.os.name
    arch = ctx.os.arch

    if "linux" in os_name:
        os = "linux"
    elif "mac" in os_name:
        os = "macos"
    elif "windows" in os_name:
        os = "windows"
    else:
        fail("Unsupported OS: " + os_name)

    if arch in ("x86_64", "amd64"):
        arch = "x86_64"
    elif arch in ("aarch64", "arm64"):
        arch = "aarch64"
    else:
        fail("Unsupported architecture: " + arch)

    return (os, arch)

def _parse_lockfile(content, wheels_storage):
    """Parse a PEP 440 direct-reference lockfile.

    Each non-blank, non-comment line has the form:
        package-name @ URL#sha256=HASH

    ${INTEGRATIONS_WHEELS_STORAGE} placeholders in URLs are substituted with wheels_storage.
    Returns a list of structs with fields: name, url, sha256, filename.
    """
    wheels = []
    for line in content.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue

        parts = line.split(" @ ", 1)
        if len(parts) != 2:
            fail("Unexpected lockfile line: " + line)

        name = parts[0].strip()
        url_with_hash = parts[1].strip().replace("${INTEGRATIONS_WHEELS_STORAGE}", wheels_storage)

        url_parts = url_with_hash.split("#sha256=", 1)
        if len(url_parts) != 2:
            fail("Missing #sha256= fragment in lockfile line: " + line)

        url = url_parts[0]
        sha256 = url_parts[1]
        filename = url.split("/")[-1]

        wheels.append(struct(
            name = name,
            url = url,
            sha256 = sha256,
            filename = filename,
        ))

    return wheels

def _agent_integrations_impl(rctx):
    os, arch = _detect_platform(rctx)
    python_version = rctx.attr.python_version
    lockfile_name = "{}-{}_{}.txt".format(os, arch, python_version)

    release_info = read_effective_release_json(rctx, rctx.attr._release_info)

    # Note: this asumes that INTEGRATIONS_CORE_VERSION is the full commit hash.
    # Any other type of git reference will fail.
    commit = release_info["dependencies"]["INTEGRATIONS_CORE_VERSION"]
    rctx.download_and_extract(
        url = "{base_url}/archive/{commit}.tar.gz".format(
            base_url = rctx.attr.base_url,
            commit = commit,
        ),
        canonical_id = commit,
        strip_prefix = "integrations-core-{}".format(commit),
    )

    lockfile_content = rctx.read(".deps/resolved/{}".format(lockfile_name))
    wheels = _parse_lockfile(lockfile_content, release_info["dependencies"]["INTEGRATIONS_WHEELS_STORAGE"])

    for wheel in wheels:
        rctx.download(
            url = wheel.url,
            output = "wheelhouse/{}".format(wheel.filename),
            sha256 = wheel.sha256,
        )

    wheel_srcs = ",".join(['"wheelhouse/{}"'.format(w.filename) for w in wheels])
    rctx.template(
        "BUILD.bazel",
        Label("//deps/agent_integrations:integrations.BUILD.bazel"),
        substitutions = {"{wheel_srcs}": wheel_srcs},
    )

    return rctx.repo_metadata(reproducible = True)

agent_integrations = repository_rule(
    implementation = _agent_integrations_impl,
    attrs = {
        "base_url": attr.string(
            default = "https://github.com/DataDog/integrations-core",
            doc = "Base URL of the repository",
        ),
        "wheels_storage": attr.string(
            mandatory = True,
            doc = "Value substituted for ${INTEGRATIONS_WHEELS_STORAGE} in wheel URLs",
        ),
        "python_version": attr.string(
            default = PYTHON_MAJOR_MINOR,
            doc = "Python version string used to select the platform lockfile",
        ),
        "_release_info": attr.label(default = "//:release.json", allow_single_file = True),
    },
    doc =
        """Retrieves the integrations repository and provides rules to install the integrations
    and their dependencies in a Python environment.""",
)
