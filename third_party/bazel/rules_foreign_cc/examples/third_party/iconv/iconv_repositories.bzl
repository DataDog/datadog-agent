"""A module defining the third party dependency iconv"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def iconv_repositories():
    maybe(
        http_archive,
        name = "iconv",
        urls = [
            "https://ftp.gnu.org/pub/gnu/libiconv/libiconv-1.16.tar.gz",
        ],
        type = "tar.gz",
        sha256 = "e6a1b1b589654277ee790cce3734f07876ac4ccfaecbee8afa0b649cf529cc04",
        strip_prefix = "libiconv-1.16",
        build_file = Label("//iconv:BUILD.iconv.bazel"),
    )

    maybe(
        http_archive,
        name = "iconv_macos",
        urls = [
            "https://opensource.apple.com/tarballs/libiconv/libiconv-59.tar.gz",
        ],
        type = "tar.gz",
        sha256 = "975f31be8eb193d5099b5fc4fc343b95c0eb83d59ffa6e5bde9454def2228a53",
        strip_prefix = "libiconv-libiconv-59/libiconv",
        build_file = Label("//iconv:BUILD.iconv.macos.bazel"),
    )
