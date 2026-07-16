"""Repository rule for the lockfile-selected cryptography source distribution."""

load("@cpython_versions//:constants.bzl", "PYTHON_MAJOR_MINOR")
load("//bazel/repo:release_json.bzl", "read_effective_release_json")
load("//deps/agent_integrations/cryptography:sdists.bzl", "CRYPTOGRAPHY_SDISTS")

_TARGET_LOCKFILE_PLATFORMS = [
    ("linux", "x86_64"),
    ("linux", "aarch64"),
    ("macos", "x86_64"),
    ("macos", "aarch64"),
    ("windows", "x86_64"),
]

_CRYPTOGRAPHY_RUST_OVERLAY_FILES = [
    "src/rust/BUILD.bazel",
    "src/rust/cargo_toml_env_vars.env",
    "src/rust/cryptography-cffi/BUILD.bazel",
    "src/rust/cryptography-cffi/cargo_toml_env_vars.env",
    "src/rust/cryptography-crypto/BUILD.bazel",
    "src/rust/cryptography-crypto/cargo_toml_env_vars.env",
    "src/rust/cryptography-keepalive/BUILD.bazel",
    "src/rust/cryptography-keepalive/cargo_toml_env_vars.env",
    "src/rust/cryptography-key-parsing/BUILD.bazel",
    "src/rust/cryptography-key-parsing/cargo_toml_env_vars.env",
    "src/rust/cryptography-openssl/BUILD.bazel",
    "src/rust/cryptography-openssl/cargo_toml_env_vars.env",
    "src/rust/cryptography-x509-verification/BUILD.bazel",
    "src/rust/cryptography-x509-verification/cargo_toml_env_vars.env",
    "src/rust/cryptography-x509/BUILD.bazel",
    "src/rust/cryptography-x509/cargo_toml_env_vars.env",
]

_HEX_DIGITS = "0123456789abcdefABCDEF"

def _is_full_commit_hash(ref):
    """Returns whether ref is a 40-character Git commit hash."""
    if len(ref) != 40:
        return False

    for char in ref.elems():
        if char not in _HEX_DIGITS:
            return False

    return True

def _lockfile_name(os, arch, python_version):
    return "{}-{}_{}.txt".format(os, arch, python_version)

def _parse_lockfile(content, wheels_storage):
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
        wheels.append(struct(
            name = name,
            filename = url.split("/")[-1],
        ))

    return wheels

def _cryptography_version_from_lockfile(content, wheels_storage, lockfile_name):
    for wheel in _parse_lockfile(content, wheels_storage):
        name, version = wheel.filename.split("-")[:2]
        if name == "cryptography":
            return version

    fail("No cryptography wheel found in {}".format(lockfile_name))

def _template_cryptography_rust_overlay(rctx):
    """Installs cryptography Rust BUILD overlay files into the extracted sdist."""
    for path in _CRYPTOGRAPHY_RUST_OVERLAY_FILES:
        rctx.template(
            path,
            Label("//deps/agent_integrations/cryptography/overlay:{}".format(path)),
        )

def _patch_cryptography_cffi_build_script(rctx):
    """Patches cryptography-cffi build.rs to accept Bazel-provided script path."""
    path = "src/rust/cryptography-cffi/build.rs"
    content = rctx.read(path)
    old = '.arg("../../_cffi_src/build_openssl.py")'
    new = '.arg(env::var("CRYPTOGRAPHY_BUILD_OPENSSL_PY").unwrap_or_else(|_| "../../_cffi_src/build_openssl.py".to_string()))'
    if old not in content:
        fail("Could not patch {}: expected build_openssl.py invocation not found".format(path))
    rctx.file(path, content.replace(old, new))

def _integration_cryptography_source_impl(rctx):
    python_version = rctx.attr.python_version
    release_info = read_effective_release_json(rctx, rctx.attr._release_info)
    commit = release_info["dependencies"]["INTEGRATIONS_CORE_VERSION"]
    wheels_storage = release_info["dependencies"]["INTEGRATIONS_WHEELS_STORAGE"]

    versions = {}
    for os, arch in _TARGET_LOCKFILE_PLATFORMS:
        lockfile_name = _lockfile_name(os, arch, python_version)
        rctx.download(
            url = "{base_url}/raw/{commit}/.deps/resolved/{lockfile_name}".format(
                base_url = rctx.attr.base_url,
                commit = commit,
                lockfile_name = lockfile_name,
            ),
            output = "lockfiles/{}".format(lockfile_name),
        )
        version = _cryptography_version_from_lockfile(
            rctx.read("lockfiles/{}".format(lockfile_name)),
            wheels_storage,
            lockfile_name,
        )
        versions[version] = versions.get(version, []) + [lockfile_name]

    if len(versions) != 1:
        fail("cryptography versions differ across integrations lockfiles: {}".format(versions))

    cryptography_version = versions.keys()[0]
    if cryptography_version not in CRYPTOGRAPHY_SDISTS:
        fail("cryptography {} found in integrations lockfiles, but no sdist metadata is registered".format(cryptography_version))

    sdist = CRYPTOGRAPHY_SDISTS[cryptography_version]
    rctx.download_and_extract(
        url = sdist.url,
        sha256 = sdist.sha256,
        stripPrefix = "cryptography-{}".format(cryptography_version),
    )
    rctx.template(
        "BUILD.bazel",
        Label("//deps/agent_integrations/cryptography/overlay:BUILD.bazel"),
    )
    _patch_cryptography_cffi_build_script(rctx)
    _template_cryptography_rust_overlay(rctx)

    return rctx.repo_metadata(reproducible = _is_full_commit_hash(commit))

integration_cryptography_source = repository_rule(
    implementation = _integration_cryptography_source_impl,
    attrs = {
        "base_url": attr.string(
            default = "https://github.com/DataDog/integrations-core",
            doc = "Base URL of the repository",
        ),
        "python_version": attr.string(
            default = PYTHON_MAJOR_MINOR,
            doc = "Python version string used to select integrations-core lockfiles",
        ),
        "_release_info": attr.label(default = "//:release.json", allow_single_file = True),
    },
    doc = "Fetches the integrations-lockfile-selected cryptography sdist and overlays Bazel metadata.",
)
