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
    downloaded = {}

    if "linux" in rctx.os.name:
        arch = rctx.os.arch
        if arch in ("x86_64", "amd64"):
            arch = "amd64"
        elif arch in ("aarch64", "arm64"):
            arch = "arm64"
        else:
            fail("Unsupported architecture for LLVM BPF toolchain: " + arch)

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

    # Build the binary attributes only when we actually downloaded them;
    # omitting the attrs lets the label default to None on non-Linux.
    binary_attrs = ""
    if downloaded:
        binary_attrs = """    clang_bpf = "{clang}",
    llc_bpf = "{llc}",
    llvm_strip = "{strip}",""".format(
            clang = downloaded["clang-bpf"],
            llc = downloaded["llc-bpf"],
            strip = downloaded["llvm-strip"],
        )

    rctx.file("BUILD.bazel", content = """\
load("@@//bazel/toolchains/llvm_bpf:llvm_bpf.bzl", "llvm_bpf_toolchain")

exports_files(glob(["bin/*"], allow_empty = True))

llvm_bpf_toolchain(
    name = "llvm_bpf_impl",
{binary_attrs}
    version = "{version}",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "llvm_bpf_toolchain",
    target_compatible_with = ["@platforms//os:linux"],
    toolchain = ":llvm_bpf_impl",
    toolchain_type = "@@//bazel/toolchains/llvm_bpf:llvm_bpf_toolchain_type",
    visibility = ["//visibility:public"],
)
""".format(binary_attrs = binary_attrs, version = CLANG_VERSION))

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
