"""Repository rules for pinned Ubuntu kernel headers."""

_NAME = "linux_headers"

_ARCH_MAP = {
    "aarch64": "arm64",
    "amd64": "x86",
    "arm64": "arm64",
    "x86_64": "x86",
}

_BUILD = """
load("@rules_cc//cc:defs.bzl", "cc_library")

package(default_visibility = ["//visibility:public"])

filegroup(
    name = "all",
    srcs = glob(["usr/src/linux-headers-*/**"]),
)

cc_library(
    name = "linux_headers",
    hdrs = glob(["usr/src/linux-headers-*/**"]),
    includes = ["."],
)
"""

_STUB_BUILD = """
load("@rules_cc//cc:defs.bzl", "cc_library")

package(default_visibility = ["//visibility:public"])

filegroup(
    name = "all",
    srcs = [],
)

cc_library(
    name = "linux_headers",
)
"""

def _write_stub(rctx):
    rctx.file("BUILD.bazel", _STUB_BUILD)
    rctx.file("defs.bzl", 'KERNEL_HEADER_DIRS = []\nKERNEL_ARCH = ""\n')

def _extract_deb_payload(rctx, url, sha256, output):
    rctx.download_and_extract(
        url = url,
        output = output,
        sha256 = sha256,
        type = "deb",
    )
    rctx.extract(output + "/data.tar.zst")
    rctx.delete(output)

def _kernel_header_dirs(header_release, kernel_arch):
    common_root = "usr/src/linux-headers-{}".format(header_release)
    generic_root = "usr/src/linux-headers-{}-generic".format(header_release)
    common = [
        common_root + "/include",
        common_root + "/include/uapi",
        common_root + "/arch/{}/include".format(kernel_arch),
        common_root + "/arch/{}/include/uapi".format(kernel_arch),
    ]
    generic = [
        generic_root + "/include",
        generic_root + "/include/uapi",
        generic_root + "/include/generated/uapi",
        generic_root + "/arch/{}/include".format(kernel_arch),
        generic_root + "/arch/{}/include/uapi".format(kernel_arch),
        generic_root + "/arch/{}/include/generated".format(kernel_arch),
        generic_root + "/arch/{}/include/generated/uapi".format(kernel_arch),
    ]
    return common + generic

def _linux_headers_impl(rctx):
    if rctx.os.name != "linux":
        _write_stub(rctx)
        return

    target_arch = rctx.attr.target_arch or rctx.os.arch
    kernel_arch = _ARCH_MAP.get(target_arch)
    if not kernel_arch:
        fail("unsupported architecture '{}'; supported architectures: {}".format(
            target_arch,
            ", ".join(sorted(_ARCH_MAP.keys())),
        ))
    if target_arch not in rctx.attr.arch_urls or target_arch not in rctx.attr.arch_sha256s:
        fail("missing URL or checksum for '{}'".format(target_arch))

    _extract_deb_payload(
        rctx,
        rctx.attr.common_url,
        rctx.attr.common_sha256,
        "_common_deb",
    )
    _extract_deb_payload(
        rctx,
        rctx.attr.arch_urls[target_arch],
        rctx.attr.arch_sha256s[target_arch],
        "_arch_deb",
    )

    rctx.file("BUILD.bazel", _BUILD)
    rctx.file(
        "defs.bzl",
        """KERNEL_HEADER_DIRS = {header_dirs}
KERNEL_ARCH = "{kernel_arch}"
""".format(
            header_dirs = repr(_kernel_header_dirs(rctx.attr.header_release, kernel_arch)),
            kernel_arch = kernel_arch,
        ),
    )

linux_headers_repo = repository_rule(
    implementation = _linux_headers_impl,
    attrs = {
        "arch_sha256s": attr.string_dict(mandatory = True),
        "arch_urls": attr.string_dict(mandatory = True),
        "common_sha256": attr.string(mandatory = True),
        "common_url": attr.string(mandatory = True),
        "header_release": attr.string(mandatory = True),
        "target_arch": attr.string(),
    },
)

_configure = tag_class(
    attrs = {
        "arch_sha256s": attr.string_dict(mandatory = True),
        "arch_urls": attr.string_dict(mandatory = True),
        "common_sha256": attr.string(mandatory = True),
        "common_url": attr.string(mandatory = True),
        "header_release": attr.string(mandatory = True),
    },
)

def _linux_headers_ext_impl(ctx):
    configurations = [
        config
        for module in ctx.modules
        for config in module.tags.configure
    ]
    if len(configurations) != 1:
        fail("linux_headers_extension requires exactly one configure tag")

    config = configurations[0]
    for arch in ["x86_64", "aarch64"]:
        if arch not in config.arch_urls or arch not in config.arch_sha256s:
            fail("linux_headers_extension is missing URL or checksum for '{}'".format(arch))

    common = {
        "arch_sha256s": config.arch_sha256s,
        "arch_urls": config.arch_urls,
        "common_sha256": config.common_sha256,
        "common_url": config.common_url,
        "header_release": config.header_release,
    }
    linux_headers_repo(
        name = _NAME,
        **common
    )
    linux_headers_repo(
        name = _NAME + "_x86_64",
        target_arch = "x86_64",
        **common
    )
    linux_headers_repo(
        name = _NAME + "_aarch64",
        target_arch = "aarch64",
        **common
    )

    return ctx.extension_metadata(reproducible = True)

linux_headers_extension = module_extension(
    implementation = _linux_headers_ext_impl,
    tag_classes = {"configure": _configure},
)
