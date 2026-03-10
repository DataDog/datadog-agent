"""Repository rule to download LLVM/Clang for eBPF compilation from Datadog S3."""

NAME = "llvm_bpf"

_BINARIES = {
    "clang-bpf": "clang",
    "llc-bpf": "llc",
    "llvm-strip": "llvm-strip",
}

def _get_url(s3_base, url_prefix, clang_version, arch, build_version):
    return "{}/{}-{}.{}.{}".format(s3_base, url_prefix, clang_version, arch, build_version)

def _download_llvm_bpf_impl(rctx):
    downloaded = {}
    clang_version = rctx.attr.clang_version

    if "linux" in rctx.os.name:
        arch = rctx.os.arch
        if arch in ("x86_64", "amd64"):
            arch = "amd64"
        elif arch in ("aarch64", "arm64"):
            arch = "arm64"
        else:
            fail("Unsupported architecture for LLVM BPF toolchain: " + arch)

        for binary, url_prefix in _BINARIES.items():
            url = _get_url(rctx.attr.s3_base_url, url_prefix, clang_version, arch, rctx.attr.clang_build_version)
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

    binary_attrs = ""
    if downloaded:
        binary_attrs = """    clang_bpf = "{clang}",
    llc_bpf = "{llc}",
    llvm_strip = "{strip}",""".format(
            clang = downloaded["clang-bpf"],
            llc = downloaded["llc-bpf"],
            strip = downloaded["llvm-strip"],
        )

    install_target = ""
    if downloaded:
        install_target = """
load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")

pkg_files(
    name = "all_files",
    srcs = glob(["bin/*"]),
    prefix = "embedded/bin",
    attributes = pkg_attributes(mode = "0755"),
)

pkg_install(
    name = "install",
    srcs = [":all_files"],
    target_compatible_with = ["@platforms//os:linux"],
)
"""

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
{install_target}""".format(
        binary_attrs = binary_attrs,
        version = clang_version,
        install_target = install_target,
    ))

download_llvm_bpf = repository_rule(
    implementation = _download_llvm_bpf_impl,
    doc = "Download LLVM BPF toolchain binaries from Datadog S3.",
    attrs = {
        "s3_base_url": attr.string(mandatory = True),
        "clang_version": attr.string(mandatory = True),
        "clang_build_version": attr.string(mandatory = True),
        "verbose": attr.bool(default = False),
    },
)

_configure = tag_class(
    attrs = {
        "s3_base_url": attr.string(default = "https://dd-agent-omnibus.s3.amazonaws.com/llvm"),
        "clang_version": attr.string(default = "12.0.1"),
        "clang_build_version": attr.string(default = "v60409452-ee70de70"),
        "verbose": attr.bool(default = False),
    },
)

def _llvm_bpf_extension_impl(ctx):
    cfg = ctx.modules[0].tags.configure[0] if ctx.modules[0].tags.configure else struct(
        s3_base_url = "https://dd-agent-omnibus.s3.amazonaws.com/llvm",
        clang_version = "12.0.1",
        clang_build_version = "v60409452-ee70de70",
        verbose = False,
    )
    download_llvm_bpf(
        name = NAME,
        s3_base_url = cfg.s3_base_url,
        clang_version = cfg.clang_version,
        clang_build_version = cfg.clang_build_version,
        verbose = cfg.verbose,
    )

llvm_bpf_extension = module_extension(
    implementation = _llvm_bpf_extension_impl,
    tag_classes = {"configure": _configure},
)
