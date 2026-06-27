"""Repository rule to download a prebuilt bpftool binary for CO-RE BTF minimization.

Mirrors `bazel/toolchains/llvm_bpf/llvm_bpf_configure.bzl`: a repository rule
downloads a pinned, sha256-checked bpftool release and exposes it as a toolchain.

The btf-gen build image builds bpftool from source (libbpf submodule, pinned to a
git commit). For the Bazel migration we instead pull the upstream prebuilt static
release: any recent bpftool produces a correct minimized BTF (a superset of the
types referenced by the CO-RE programs), and pinning the exact bytes via sha256 is
what makes the action hermetic and cacheable. The long-term home for these binaries
is the `dd-agent-omnibus` S3 mirror used by the llvm_bpf toolchain (see PR notes);
the GitHub release URL keeps the spike self-contained until that mirror exists.
"""

NAME = "bpftool"

def _download_bpftool_impl(rctx):
    # bpftool is a Linux-only tool. On other hosts we still create a (invalid)
    # toolchain so analysis works everywhere; min_core_btf targets are
    # target_compatible_with linux and are skipped on non-Linux platforms.
    if "linux" not in rctx.os.name:
        _write_build(rctx, valid = False)
        return

    arch = rctx.os.arch
    if arch in ("x86_64", "amd64"):
        arch = "amd64"
    elif arch in ("aarch64", "arm64"):
        arch = "arm64"
    else:
        fail("Unsupported architecture for bpftool toolchain: " + arch)

    url = "{}/bpftool-{}-{}.tar.gz".format(rctx.attr.base_url, rctx.attr.version, arch)
    sha256 = rctx.attr.sha256s.get(arch, "")

    if rctx.attr.verbose:
        # buildifier: disable=print
        print("Downloading bpftool from {}".format(url))

    # The release tarball contains a single top-level `bpftool` binary.
    result = rctx.download_and_extract(url = url, sha256 = sha256)
    if not result.success:
        fail("Failed to download bpftool from {}".format(url))

    # download_and_extract preserves tar permissions (the release binary is 0755),
    # but chmod defensively so the executable label attr always resolves.
    rctx.execute(["chmod", "0755", "bpftool"])

    _write_build(rctx, valid = True)

def _write_build(rctx, valid):
    if valid:
        bpftool_attr = '    bpftool = "bpftool",'
    else:
        bpftool_attr = ""

    rctx.file("BUILD.bazel", content = """\
load("@@//bazel/toolchains/bpftool:bpftool.bzl", "bpftool_toolchain")

exports_files(glob(["bpftool"], allow_empty = True))

bpftool_toolchain(
    name = "bpftool_impl",
{bpftool_attr}
    version = "{version}",
    visibility = ["//visibility:public"],
)

toolchain(
    name = "bpftool_toolchain",
    target_compatible_with = ["@platforms//os:linux"],
    toolchain = ":bpftool_impl",
    toolchain_type = "@@//bazel/toolchains/bpftool:bpftool_toolchain_type",
    visibility = ["//visibility:public"],
)
""".format(
        bpftool_attr = bpftool_attr,
        version = rctx.attr.version,
    ))

download_bpftool = repository_rule(
    implementation = _download_bpftool_impl,
    doc = "Download a prebuilt bpftool binary from a pinned release.",
    attrs = {
        "base_url": attr.string(mandatory = True),
        "version": attr.string(mandatory = True),
        "sha256s": attr.string_dict(doc = "SHA256 checksums keyed by arch ('amd64', 'arm64')."),
        "verbose": attr.bool(default = False),
    },
)

_DEFAULTS = struct(
    base_url = "https://github.com/libbpf/bpftool/releases/download/v7.5.0",
    version = "v7.5.0",
    sha256s = {},
    verbose = False,
)

_configure = tag_class(
    attrs = {
        "base_url": attr.string(default = _DEFAULTS.base_url),
        "version": attr.string(default = _DEFAULTS.version),
        "sha256s": attr.string_dict(doc = "SHA256 checksums keyed by arch ('amd64', 'arm64')."),
        "verbose": attr.bool(default = False),
    },
)

def _bpftool_extension_impl(ctx):
    cfg = ctx.modules[0].tags.configure[0] if ctx.modules[0].tags.configure else _DEFAULTS
    download_bpftool(
        name = NAME,
        base_url = cfg.base_url,
        version = cfg.version,
        sha256s = cfg.sha256s,
        verbose = cfg.verbose,
    )

bpftool_extension = module_extension(
    implementation = _bpftool_extension_impl,
    tag_classes = {"configure": _configure},
)
