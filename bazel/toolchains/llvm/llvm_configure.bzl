"""Repository rule to download and configure LLVM/Clang toolchain for eBPF compilation."""

NAME = "llvm_toolchain"

# These match the versions from .gitlab-ci.yml
CLANG_VERSION = "12.0.1"
CLANG_BUILD_VERSION = "v60409452-ee70de70"

def _get_llvm_url(url_prefix, arch):
    """Generate URL for downloading LLVM binaries from Datadog S3."""
    return "https://dd-agent-omnibus.s3.amazonaws.com/llvm/{}-{}.{}.{}".format(
        url_prefix,
        CLANG_VERSION,
        arch,
        CLANG_BUILD_VERSION,
    )

def _write_build(rctx, binaries):
    """Write BUILD file with toolchain definitions."""
    rctx.template(
        "BUILD.bazel",
        Label("@@//bazel/toolchains/llvm:BUILD.tpl"),
        substitutions = {
            "{GENERATOR}": "@@//bazel/toolchains/llvm/llvm_configure.bzl%download_llvm_toolchain",
            "{CLANG_VERSION}": CLANG_VERSION,
            "{CLANG_BPF_PATH}": binaries.get("clang-bpf", ""),
            "{LLC_BPF_PATH}": binaries.get("llc-bpf", ""),
            "{LLVM_STRIP_PATH}": binaries.get("llvm-strip", ""),
        },
        executable = False,
    )

def _download_llvm_toolchain_impl(rctx):
    """Download LLVM/Clang binaries from Datadog S3."""

    # Detect architecture - use Datadog's arch naming (amd64, arm64)
    arch = rctx.os.arch
    if arch == "x86_64":
        arch = "amd64"
    elif arch == "aarch64":
        arch = "arm64"

    # Map binary names to their S3 URL prefixes
    binaries_to_download = {
        "clang-bpf": "clang",
        "llc-bpf": "llc",
        "llvm-strip": "llvm-strip",
    }
    downloaded_binaries = {}

    for binary, url_prefix in binaries_to_download.items():
        url = _get_llvm_url(url_prefix, arch)
        output = "bin/{}".format(binary)

        if rctx.attr.verbose:
            print("Downloading {} from {}".format(binary, url))  # buildifier: disable=print

        result = rctx.download(
            url = url,
            output = output,
            executable = True,
        )

        if result.success:
            downloaded_binaries[binary] = output
            if rctx.attr.verbose:
                print("Successfully downloaded {}".format(binary))  # buildifier: disable=print
        else:
            fail("Failed to download {} from {}".format(binary, url))

    _write_build(rctx, downloaded_binaries)

download_llvm_toolchain = repository_rule(
    implementation = _download_llvm_toolchain_impl,
    doc = """Download LLVM/Clang toolchain for eBPF compilation from Datadog S3.""",
    attrs = {
        "verbose": attr.bool(
            doc = "If true, print download status messages.",
            default = False,
        ),
    },
)

llvm_toolchain_extension = module_extension(
    implementation = lambda ctx: download_llvm_toolchain(name = NAME),
)
