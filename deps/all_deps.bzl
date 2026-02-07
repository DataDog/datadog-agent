"""This is the source of truth for all source archives we use for the agent BUILD.

Usage:
    load("@@//deps:all_deps.bzl", "ALL_DEPS")

    info = ALL_DEPS[repo_name]

    ... info contains the fiels name, version, sha256, strip_prefix, urls
"""

DEFAULT_STRIP_PREFIX = "{name}-{version}"

def _source_archive(name = None, version = None, sha256 = None, strip_prefix = None, urls = None, so_version = None):
    """Makes a struct of information about a library

    Args:
        name: package name
        version: package version
        sha256: (optional) checksum to verify download
        strip_prefix: (optional) prefix to strip when un-tarring.
        urls: (optional) Download paths
        so_version: (optional) API version number for dynamic libraires
    """
    ret = {
        "name": name,
        "version": version,
    }
    if sha256:
        ret["sha256"] = sha256
    if strip_prefix:
        ret["strip_prefix"] = strip_prefix.format(name = name, version = version)
    if urls:
        ret["urls"] = [url.format(name = name, version = version) for url in urls]
    if so_version:
        ret["so_version"] = so_version
    if version not in ret.get("strip_prefix") and version not in ret.get("urls", [])[0]:
        fail("%s version:%s does not appear in strip_prefix or urls" % (name, version))
    return struct(**ret)

ALL_SOURCES = [
    _source_archive(
        name = "xz",
        version = "5.8.1",
        sha256 = "507825b599356c10dca1cd720c9d0d0c9d5400b9de300af00e4d1ea150795543",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = [
            "https://dd-agent-omnibus.s3.amazonaws.com/bazel/xz-{version}.tar.gz",
            "https://tukaani.org/xz/xz-{version}.tar.gz",
        ],
    ),
    _source_archive(
        name = "zlib",
        version = "1.3.1",
        sha256 = "9a93b2b7dfdac77ceba5a558a580e74667dd6fede4585b91eefb60f03b72df23",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = [
            "https://dd-agent-omnibus.s3.amazonaws.com/bazel/zlib-{version}.tar.gz",
            "https://github.com/madler/zlib/releases/download/v{version}/zlib-{version}.tar.gz",
        ],
    ),
    _source_archive(
        name = "bzip2",
        version = "1.0.8",
        sha256 = "ab5a03176ee106d3f0fa90e381da478ddae405918153cca248e682cd0c4a2269",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        # The canonical source is in https://sourceware.org/pub/bzip2/, but the Bazel
        # team mirror is more reliable.
        urls = ["https://mirror.bazel.build/sourceware.org/pub/bzip2/bzip2-{version}.tar.gz"],
    ),
    _source_archive(
        name = "openssl",
        version = "3.5.4",
        sha256 = "967311f84955316969bdb1d8d4b983718ef42338639c621ec4c34fddef355e99",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://www.openssl.org/source/openssl-{version}.tar.gz"],
    ),
    _source_archive(
        name = "libffi",
        version = "3.4.8",
        sha256 = "bc9842a18898bfacb0ed1252c4febcc7e78fa139fd27fdc7a3e30d9d9356119b",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://github.com/libffi/libffi/releases/download/v{version}/libffi-{version}.tar.gz"],
    ),
    _source_archive(
        name = "libsepol",
        version = "3.5",
        sha256 = "78fdaf69924db780bac78546e43d9c44074bad798c2c415d0b9bb96d065ee8a2",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://github.com/SELinuxProject/selinux/releases/download/{version}/libsepol-{version}.tar.gz"],
    ),

    # PCRE2 has native bazel support.
    # They expose a :pcre2 label for the main library
    _source_archive(
        name = "pcre2",
        version = "10.46",
        sha256 = "15fbc5aba6beee0b17aecb04602ae39432393aba1ebd8e39b7cabf7db883299f",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://github.com/PCRE2Project/pcre2/releases/download/pcre2-{version}/pcre2-{version}.tar.bz2"],
    ),
    _source_archive(
        name = "util-linux",
        version = "2.39.2",
        sha256 = "c8e1a11dd5879a2788973c73589fbcf08606e85aeec095e516162495ead8ba68",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://mirrors.edge.kernel.org/pub/linux/utils/util-linux/v2.39/util-linux-{version}.tar.gz"],
    ),
    _source_archive(
        name = "libselinux",
        version = "3.5",
        sha256 = "9a3a3705ac13a2ccca2de6d652b6356fead10f36fb33115c185c5ccdf29eec19",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://github.com/SELinuxProject/selinux/releases/download/{version}/libselinux-{version}.tar.gz"],
    ),
    _source_archive(
        name = "patchelf",
        version = "0.18.0",
        sha256 = "64de10e4c6b8b8379db7e87f58030f336ea747c0515f381132e810dbf84a86e7",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://github.com/NixOS/patchelf/releases/download/{version}/patchelf-{version}.tar.gz"],
    ),
    _source_archive(
        name = "nghttp2",
        version = "1.58.0",
        sha256 = "9ebdfbfbca164ef72bdf5fd2a94a4e6dfb54ec39d2ef249aeb750a91ae361dfb",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://github.com/nghttp2/nghttp2/releases/download/v{version}/nghttp2-{version}.tar.gz"],
    ),
    _source_archive(
        name = "popt",
        version = "1.19",
        sha256 = "c25a4838fc8e4c1c8aacb8bd620edb3084a3d63bf8987fdad3ca2758c63240f9",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["http://ftp.rpm.org/popt/releases/popt-1.x/popt-{version}.tar.gz"],
    ),
    _source_archive(
        name = "libyaml",
        version = "0.2.5",
        sha256 = "c642ae9b75fee120b2d96c712538bd2cf283228d2337df2cf2988e3c02678ef4",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://pyyaml.org/download/libyaml/yaml-{version}.tar.gz"],
    ),
    _source_archive(
        name = "attr",
        version = "2.5.1",
        sha256 = "db448a626f9313a1a970d636767316a8da32aede70518b8050fa0de7947adc32",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = [
            "https://dd-agent-omnibus.s3.amazonaws.com/bazel/attr-attr-{version}.tar.xz",
            "https://download.savannah.nongnu.org/releases/attr/attr-{version}.tar.xz",
        ],
    ),
    _source_archive(
        name = "zstd",
        version = "1.5.7",
        sha256 = "eb33e51f49a15e023950cd7825ca74a4a2b43db8354825ac24fc1b7ee09e6fa3",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://github.com/facebook/zstd/releases/download/v{version}/zstd-{version}.tar.gz"],
    ),
    _source_archive(
        name = "gpg-error",
        version = "1.56",
        sha256 = "82c3d2deb4ad96ad3925d6f9f124fe7205716055ab50e291116ef27975d169c0",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://www.gnupg.org/ftp/gcrypt/libgpg-error/libgpg-error-{version}.tar.bz2"],
    ),
    _source_archive(
        name = "gcrypt",
        version = "1.11.2",
        sha256 = "6ba59dd192270e8c1d22ddb41a07d95dcdbc1f0fb02d03c4b54b235814330aac",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://gnupg.org/ftp/gcrypt/libgcrypt/libgcrypt-{version}.tar.bz2"],
    ),
    _source_archive(
        name = "unixodbc",
        version = "2.3.9",
        sha256 = "52833eac3d681c8b0c9a5a65f2ebd745b3a964f208fc748f977e44015a31b207",
        strip_prefix = DEFAULT_STRIP_PREFIX,
        urls = ["https://www.unixodbc.org/unixODBC-{version}.tar.gz"],
    ),
]

ALL_DEPS = {x.name: x for x in ALL_SOURCES}
