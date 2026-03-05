"""Repository rule to download LLVM/Clang for eBPF compilation from Datadog S3."""

NAME = "llvm_bpf"

CLANG_VERSION = "12.0.1"
CLANG_BUILD_VERSION = "v60409452-ee70de70"

_S3_BASE = "https://dd-agent-omnibus.s3.amazonaws.com/llvm"

_BINARIES = {
    "clang-bpf": "clang",
    "llc-bpf": "llc",
    "llvm-strip": "llvm-strip",
}

def _get_url(url_prefix, arch):
    return "{}/{}-{}.{}.{}".format(_S3_BASE, url_prefix, CLANG_VERSION, arch, CLANG_BUILD_VERSION)

def _download_llvm_bpf_impl(rctx):
    arch = rctx.os.arch
    if arch == "x86_64":
        arch = "amd64"
    elif arch == "aarch64":
        arch = "arm64"
    else:
        fail("Unsupported architecture for LLVM BPF toolchain: " + arch)

    downloaded = {}
    for binary, url_prefix in _BINARIES.items():
        url = _get_url(url_prefix, arch)
        output = "bin/" + binary

        if rctx.attr.verbose:
            # buildifier: disable=print
            print("Downloading {} from {}".format(binary, url))

        result = rctx.download(
            url = url,
            output = output,
            executable = True,
        )
        if not result.success:
            fail("Failed to download {} from {}".format(binary, url))
        downloaded[binary] = output

    rctx.template(
        "BUILD.bazel",
        Label("@@//bazel/toolchains/llvm_bpf:BUILD.tpl"),
        substitutions = {
            "{GENERATOR}": "@@//bazel/toolchains/llvm_bpf/llvm_bpf_configure.bzl%llvm_bpf_extension",
            "{CLANG_VERSION}": CLANG_VERSION,
            "{CLANG_BPF_PATH}": downloaded.get("clang-bpf", ""),
            "{LLC_BPF_PATH}": downloaded.get("llc-bpf", ""),
            "{LLVM_STRIP_PATH}": downloaded.get("llvm-strip", ""),
        },
        executable = False,
    )

download_llvm_bpf = repository_rule(
    implementation = _download_llvm_bpf_impl,
    doc = "Download LLVM BPF toolchain binaries from Datadog S3.",
    attrs = {
        "verbose": attr.bool(default = False),
    },
)

llvm_bpf_extension = module_extension(
    implementation = lambda ctx: download_llvm_bpf(name = NAME),
)
