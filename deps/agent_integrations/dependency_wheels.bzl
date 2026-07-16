"""Repository rule to retrieve target-platform integration dependency wheels."""

load("@cpython_versions//:constants.bzl", "PYTHON_MAJOR_MINOR")
load("//bazel/repo:release_json.bzl", "read_effective_release_json")
load("//deps/agent_integrations/cryptography:sdists.bzl", "CRYPTOGRAPHY_SDISTS")

def _lockfile_name(os, arch, python_version):
    """Returns the target-platform lockfile name."""
    if os not in ("linux", "macos", "windows"):
        fail("Unsupported OS: " + os)

    if arch not in ("x86_64", "aarch64"):
        fail("Unsupported architecture: " + arch)

    return "{}-{}_{}.txt".format(os, arch, python_version)

_HEX_DIGITS = "0123456789abcdefABCDEF"

def _is_full_commit_hash(ref):
    """Returns whether ref is a 40-character Git commit hash."""
    if len(ref) != 40:
        return False

    for char in ref.elems():
        if char not in _HEX_DIGITS:
            return False

    return True

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

def _wheel_platform_tag(os, arch):
    """Returns the wheel platform tag for a target OS/architecture pair."""
    if os == "linux" and arch == "x86_64":
        return "manylinux_2_17_x86_64.manylinux2014_x86_64"
    if os == "linux" and arch == "aarch64":
        return "manylinux_2_17_aarch64.manylinux2014_aarch64"
    if os == "macos" and arch == "x86_64":
        return "macosx_10_12_x86_64"
    if os == "macos" and arch == "aarch64":
        return "macosx_11_0_arm64"
    if os == "windows" and arch == "x86_64":
        return "win_amd64"

    fail("Unsupported wheel platform: {} {}".format(os, arch))

def _extension_filename(os):
    """Returns the cryptography native extension basename for a target OS."""
    if os == "windows":
        return "_rust.pyd"
    return "_rust.abi3.so"

def _integration_dependency_wheels_impl(rctx):
    python_version = rctx.attr.python_version
    lockfile_name = _lockfile_name(rctx.attr.os, rctx.attr.arch, python_version)

    release_info = read_effective_release_json(rctx, rctx.attr._release_info)

    commit = release_info["dependencies"]["INTEGRATIONS_CORE_VERSION"]
    reproducible = _is_full_commit_hash(commit)
    rctx.download(
        url = "{base_url}/raw/{commit}/.deps/resolved/{lockfile_name}".format(
            base_url = rctx.attr.base_url,
            commit = commit,
            lockfile_name = lockfile_name,
        ),
        output = lockfile_name,
    )

    lockfile_content = rctx.read(lockfile_name)
    wheels = _parse_lockfile(lockfile_content, release_info["dependencies"]["INTEGRATIONS_WHEELS_STORAGE"])

    # Pick out cryptography and its version for rebuilding it on FIPS.
    # TODO: Make this replacement FIPS-specific rather than unconditional.
    cryptography_wheel, cryptography_version = None, None
    for wheel in wheels:
        name, version = wheel.filename.split("-")[:2]
        if name == "cryptography":
            cryptography_wheel = wheel
            cryptography_version = version
            break

    if not cryptography_wheel:
        fail("No cryptography wheel found in {}".format(lockfile_name))

    wheels_without_cryptography = [wheel for wheel in wheels if wheel != cryptography_wheel]

    if cryptography_version not in CRYPTOGRAPHY_SDISTS:
        fail("cryptography {} found in {}, but no sdist metadata is registered".format(
            cryptography_version,
            lockfile_name,
        ))

    for wheel in wheels:
        rctx.download(
            url = wheel.url,
            output = "wheelhouse/{}".format(wheel.filename),
            sha256 = wheel.sha256,
        )

    wheel_srcs = ",".join(['"wheelhouse/{}"'.format(w.filename) for w in wheels_without_cryptography])
    rctx.template(
        "BUILD.bazel",
        Label("//deps/agent_integrations:dependency_wheels.BUILD.bazel.tmpl"),
        substitutions = {
            "{abi_tag}": "abi3",
            "{cryptography_version}": cryptography_version,
            "{cryptography_wheel_src}": '"wheelhouse/{}"'.format(cryptography_wheel.filename),
            "{extension_filename}": _extension_filename(rctx.attr.os),
            "{platform_tag}": _wheel_platform_tag(rctx.attr.os, rctx.attr.arch),
            "{python_tag}": "cp39",
            "{wheel_srcs}": wheel_srcs,
        },
    )

    return rctx.repo_metadata(reproducible = reproducible)

integration_dependency_wheels = repository_rule(
    implementation = _integration_dependency_wheels_impl,
    attrs = {
        "base_url": attr.string(
            default = "https://github.com/DataDog/integrations-core",
            doc = "Base URL of the repository",
        ),
        "os": attr.string(
            mandatory = True,
            doc = "Target OS used to select the platform lockfile: linux, macos, or windows",
        ),
        "arch": attr.string(
            mandatory = True,
            doc = "Target architecture used to select the platform lockfile: x86_64 or aarch64",
        ),
        "python_version": attr.string(
            default = PYTHON_MAJOR_MINOR,
            doc = "Python version string used to select the platform lockfile",
        ),
        "_release_info": attr.label(default = "//:release.json", allow_single_file = True),
    },
    doc =
        """Retrieves integration dependency wheels for a target platform and provides rules to
    install them in a Python environment.""",
)
