"""Repository rule to download LLVM/Clang for eBPF compilation from Datadog S3.

The llvm_bpf_extension generates llvm_bpf_amd64 / llvm_bpf_arm64, which download
clang-bpf and llvm-strip for a single arch each, with different selection axes:

- packaging: clang-bpf embedded in agent packages runs on the target machine
  for runtime eBPF compilation, so it must match the target CPU, never the
  host's.

- toolchain: both binaries make up the LLVM BPF toolchain used to compile eBPF
  programs at build time, matching the exec platform's CPU. Its toolchain()
  targets live in //bazel/toolchains/llvm_bpf: declared here, registering them
  would fetch both repos.

- installation: both binaries are also installed on build machines and in KMT
  VMs of either arch.
"""

# Mapping from standardized arch name to @platforms//cpu arch name
_ARCH_TO_CPU = {
    "amd64": "x86_64",
    "arm64": "arm64",
}

def _get_url(s3_base, url_prefix, clang_version, arch, build_version):
    return "{}/{}-{}.{}.{}".format(s3_base, url_prefix, clang_version, arch, build_version)

def _download_llvm_bpf_impl(rctx):
    for binary in ("clang-bpf", "llvm-strip"):
        url_prefix = binary.removesuffix("-bpf")  # clang-bpf is published as clang
        url = _get_url(
            rctx.attr.s3_base_url,
            url_prefix,
            rctx.attr.clang_version,
            rctx.attr.arch,
            rctx.attr.clang_build_version,
        )

        rctx.report_progress("Downloading {} ({}) from {}".format(binary, rctx.attr.arch, url))

        result = rctx.download(
            url = url,
            output = "bin/{}".format(binary),
            executable = True,
            sha256 = rctx.attr.sha256s["{}.{}".format(url_prefix, rctx.attr.arch)],
        )
        if not result.success:
            fail("Failed to download {} for {} from {}".format(binary, rctx.attr.arch, url))

    rctx.file("BUILD.bazel", content = """\
load("@@//bazel/toolchains/llvm_bpf:llvm_bpf.bzl", "llvm_bpf_toolchain")
load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")

llvm_bpf_toolchain(
    name = "llvm_bpf",
    clang_bpf = "bin/clang-bpf",
    llvm_strip = "bin/llvm-strip",
    version = "{version}",
    visibility = ["//visibility:public"],
)

filegroup(
    name = "clang-bpf",
    srcs = ["bin/clang-bpf"],
    target_compatible_with = [
        "@platforms//os:linux",
        "@platforms//cpu:{cpu}",
    ],
    visibility = ["//visibility:public"],
)

pkg_files(
    name = "all_files",
    srcs = ["bin/clang-bpf", "bin/llvm-strip"],
    attributes = pkg_attributes(mode = "0755"),
)

pkg_install(
    name = "install",
    srcs = [":all_files"],
    visibility = ["//visibility:public"],
)
""".format(cpu = _ARCH_TO_CPU[rctx.attr.arch], version = rctx.attr.clang_version))

download_llvm_bpf = repository_rule(
    implementation = _download_llvm_bpf_impl,
    doc = "Download clang-bpf and llvm-strip for one target arch from Datadog S3.",
    attrs = {
        "s3_base_url": attr.string(mandatory = True),
        "clang_version": attr.string(mandatory = True),
        "clang_build_version": attr.string(mandatory = True),
        "arch": attr.string(mandatory = True, doc = "arch of the binaries, e.g. 'amd64'"),
        "sha256s": attr.string_dict(
            mandatory = True,
            doc = "SHA256 checksums keyed by '{binary_prefix}.{arch}', e.g. 'clang.amd64'.",
        ),
    },
)

_configure = tag_class(
    attrs = {
        "s3_base_url": attr.string(mandatory = True),
        "clang_version": attr.string(mandatory = True),
        "clang_build_version": attr.string(mandatory = True),
        "sha256s": attr.string_dict(
            mandatory = True,
            doc = "SHA256 checksums keyed by '{binary_prefix}.{arch}', e.g. 'clang.amd64'.",
        ),
    },
)

def _llvm_bpf_extension_impl(ctx):
    cfg = ctx.modules[0].tags.configure[0]
    repos = {"llvm_bpf_{}".format(arch): arch for arch in _ARCH_TO_CPU.keys()}
    for repo_name, arch in repos.items():
        download_llvm_bpf(
            name = repo_name,
            s3_base_url = cfg.s3_base_url,
            clang_version = cfg.clang_version,
            clang_build_version = cfg.clang_build_version,
            arch = arch,
            sha256s = cfg.sha256s,
        )

    return ctx.extension_metadata(
        reproducible = True,
        root_module_direct_deps = repos.keys(),
        root_module_direct_dev_deps = [],
    )

llvm_bpf_extension = module_extension(
    implementation = _llvm_bpf_extension_impl,
    tag_classes = {"configure": _configure},
)
