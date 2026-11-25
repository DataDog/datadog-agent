"""Repository rules to download LLVM/Clang for eBPF compilation from Datadog S3.

The llvm_bpf_extension generates a set of repositories with different
selection axes:

- llvm_bpf_amd64 / llvm_bpf_arm64 (packaging repos): download clang-bpf for
  a single *target* arch each. clang-bpf embedded in agent packages runs on
  the target machine for runtime eBPF compilation, so it must match the target
  CPU, never the host's.

- llvm_bpf (toolchain repo): downloads llvm-strip for the machine running
  Bazel and takes clang-bpf from the matching arch-specific repo, registering
  both as the LLVM BPF toolchain used to compile eBPF programs at build time.
"""

load("@bazel_lib//lib:repo_utils.bzl", "repo_utils")

NAME = "llvm_bpf"

# Mapping from standardized arch name to @platforms//cpu arch name
_ARCH_TO_CPU = {
    "amd64": "x86_64",
    "arm64": "arm64",
}

def _get_url(s3_base, url_prefix, clang_version, arch, build_version):
    return "{}/{}-{}.{}.{}".format(s3_base, url_prefix, clang_version, arch, build_version)

def _download_llvm_bpf_impl(rctx):
    clang_version = rctx.attr.clang_version
    binary_attrs = ""
    install_target = ""
    exec_compatible_with = ""

    if "linux" in rctx.os.name:
        arch = repo_utils.arch(rctx)
        if arch not in _ARCH_TO_CPU:
            fail("Unsupported architecture for LLVM BPF toolchain: " + arch)

        url = _get_url(rctx.attr.s3_base_url, "llvm-strip", clang_version, arch, rctx.attr.clang_build_version)
        sha256 = rctx.attr.sha256s.get("llvm-strip.{}".format(arch), "")

        rctx.report_progress("Downloading llvm-strip from {}".format(url))

        result = rctx.download(
            url = url,
            output = "bin/llvm-strip",
            executable = True,
            sha256 = sha256,
        )
        if not result.success:
            fail("Failed to download llvm-strip from {}".format(url))

        # Stay in this block: buildifier treats arch as uninitialized if it
        # is read after the Linux-only assignment.
        exec_compatible_with = """    exec_compatible_with = [
        "@platforms//os:linux",
        "@platforms//cpu:{cpu}",
    ],
""".format(cpu = _ARCH_TO_CPU[arch])
        binary_attrs = """    clang_bpf = "@llvm_bpf_{arch}//:bin/clang-bpf",
    llvm_strip = "bin/llvm-strip",""".format(arch = arch)
        install_target = """
load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")

pkg_files(
    name = "all_files",
    srcs = glob(["bin/*"]) + ["@llvm_bpf_{arch}//:bin/clang-bpf"],
    prefix = "embedded/bin",
    attributes = pkg_attributes(mode = "0755"),
)

pkg_install(
    name = "install",
    srcs = [":all_files"],
    target_compatible_with = ["@platforms//os:linux"],
)
""".format(arch = arch)

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
{exec_compatible_with}    target_compatible_with = ["@platforms//os:linux"],
    toolchain = ":llvm_bpf_impl",
    toolchain_type = "@@//bazel/toolchains/llvm_bpf:llvm_bpf_toolchain_type",
    visibility = ["//visibility:public"],
)
{install_target}""".format(
        binary_attrs = binary_attrs,
        exec_compatible_with = exec_compatible_with,
        version = clang_version,
        install_target = install_target,
    ))

download_llvm_bpf = repository_rule(
    implementation = _download_llvm_bpf_impl,
    doc = "Download LLVM BPF toolchain binaries (host arch) from Datadog S3.",
    attrs = {
        "s3_base_url": attr.string(mandatory = True),
        "clang_version": attr.string(mandatory = True),
        "clang_build_version": attr.string(mandatory = True),
        "sha256s": attr.string_dict(doc = "SHA256 checksums keyed by '{binary_prefix}.{arch}', e.g. 'clang.amd64'."),
    },
)

def _download_llvm_bpf_clang_impl(rctx):
    url = _get_url(
        rctx.attr.s3_base_url,
        "clang",
        rctx.attr.clang_version,
        rctx.attr.arch,
        rctx.attr.clang_build_version,
    )

    rctx.report_progress("Downloading clang-bpf ({}) from {}".format(rctx.attr.arch, url))

    result = rctx.download(
        url = url,
        output = "bin/clang-bpf",
        executable = True,
        sha256 = rctx.attr.sha256,
    )
    if not result.success:
        fail("Failed to download clang-bpf for {} from {}".format(rctx.attr.arch, url))

    rctx.file("BUILD.bazel", content = """\
exports_files(
    ["bin/clang-bpf"],
    visibility = ["@{toolchain}//:__pkg__"],
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
""".format(cpu = _ARCH_TO_CPU[rctx.attr.arch], toolchain = NAME))

download_llvm_bpf_clang = repository_rule(
    implementation = _download_llvm_bpf_clang_impl,
    doc = "Download clang-bpf for one target arch from Datadog S3.",
    attrs = {
        "s3_base_url": attr.string(mandatory = True),
        "clang_version": attr.string(mandatory = True),
        "clang_build_version": attr.string(mandatory = True),
        "arch": attr.string(mandatory = True, doc = "arch of the clang-bpf binary, e.g. 'amd64'"),
        "sha256": attr.string(doc = "sha256 of the clang-bpf binary"),
    },
)

_configure = tag_class(
    attrs = {
        "s3_base_url": attr.string(default = "https://dd-agent-omnibus.s3.amazonaws.com/llvm"),
        "clang_version": attr.string(default = "12.0.1"),
        "clang_build_version": attr.string(default = "v150220922-61ce1181"),
        "sha256s": attr.string_dict(doc = "SHA256 checksums keyed by '{binary_prefix}.{arch}', e.g. 'clang.amd64'."),
    },
)

def _llvm_bpf_extension_impl(ctx):
    cfg = ctx.modules[0].tags.configure[0] if ctx.modules[0].tags.configure else struct(
        s3_base_url = "https://dd-agent-omnibus.s3.amazonaws.com/llvm",
        clang_version = "12.0.1",
        clang_build_version = "v150220922-61ce1181",
        sha256s = {},
    )
    download_llvm_bpf(
        name = NAME,
        s3_base_url = cfg.s3_base_url,
        clang_version = cfg.clang_version,
        clang_build_version = cfg.clang_build_version,
        sha256s = cfg.sha256s,
    )
    repos = {"llvm_bpf_{}".format(arch): arch for arch in _ARCH_TO_CPU.keys()}
    for repo_name, arch in repos.items():
        download_llvm_bpf_clang(
            name = repo_name,
            s3_base_url = cfg.s3_base_url,
            clang_version = cfg.clang_version,
            clang_build_version = cfg.clang_build_version,
            arch = arch,
            sha256 = cfg.sha256s.get("clang.{}".format(arch)),
        )

    return ctx.extension_metadata(
        reproducible = True,
        root_module_direct_deps = [NAME] + repos.keys(),
        root_module_direct_dev_deps = [],
    )

llvm_bpf_extension = module_extension(
    implementation = _llvm_bpf_extension_impl,
    tag_classes = {"configure": _configure},
)
