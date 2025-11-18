# buildifier: disable=module-docstring
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def autotools_repositories():
    """Load all repositories needed for autotools"""

    maybe(
        http_archive,
        name = "m4",
        build_file = Label("//autotools:BUILD.m4.bazel"),
        strip_prefix = "m4-1.4.19",
        urls = [
            "https://mirror.bazel.build/ftp.gnu.org/gnu/m4/m4-1.4.19.tar.xz",
            "https://ftp.gnu.org/gnu/m4/m4-1.4.19.tar.xz",
        ],
        sha256 = "63aede5c6d33b6d9b13511cd0be2cac046f2e70fd0a07aa9573a04a82783af96",
    )

    maybe(
        http_archive,
        name = "autoconf",
        build_file = Label("//autotools:BUILD.autoconf.bazel"),
        strip_prefix = "autoconf-2.71",
        urls = [
            "https://mirror.bazel.build/ftp.gnu.org/gnu/autoconf/autoconf-2.71.tar.gz",
            "https://ftp.gnu.org/gnu/autoconf/autoconf-2.71.tar.gz",
        ],
        sha256 = "431075ad0bf529ef13cb41e9042c542381103e80015686222b8a9d4abef42a1c",
    )

    maybe(
        http_archive,
        name = "automake",
        build_file = Label("//autotools:BUILD.automake.bazel"),
        strip_prefix = "automake-1.16.4",
        urls = [
            "https://mirror.bazel.build/ftp.gnu.org/gnu/automake/automake-1.16.4.tar.gz",
            "https://ftp.gnu.org/gnu/automake/automake-1.16.4.tar.gz",
        ],
        sha256 = "8a0f0be7aaae2efa3a68482af28e5872d8830b9813a6a932a2571eac63ca1794",
    )

    maybe(
        http_archive,
        name = "libtool",
        build_file = Label("//autotools:BUILD.libtool.bazel"),
        strip_prefix = "libtool-2.4.6",
        urls = [
            "https://mirror.bazel.build/ftpmirror.gnu.org/libtool/libtool-2.4.6.tar.gz",
            "https://ftpmirror.gnu.org/libtool/libtool-2.4.6.tar.gz",
        ],
        sha256 = "e3bd4d5d3d025a36c21dd6af7ea818a2afcd4dfc1ea5a17b39d7854bcd0c06e3",
    )
