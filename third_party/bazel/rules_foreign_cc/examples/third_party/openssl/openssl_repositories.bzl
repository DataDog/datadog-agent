"""A module defining the third party dependency OpenSSL"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def openssl_repositories():
    maybe(
        http_archive,
        name = "openssl",
        build_file = Label("//openssl:BUILD.openssl.bazel"),
        integrity = "sha256-zzCYlQy02FOtlcCEHx+cbT3BAtzPys1SHZOSUgi3asg=",
        strip_prefix = "openssl-1.1.1w",
        urls = [
            "https://mirror.bazel.build/www.openssl.org/source/openssl-1.1.1w.tar.gz",
            "https://www.openssl.org/source/openssl-1.1.1w.tar.gz",
            "https://github.com/openssl/openssl/archive/OpenSSL_1_1_1w.tar.gz",
        ],
    )

    maybe(
        http_archive,
        name = "nasm",
        build_file = Label("//openssl:BUILD.nasm.bazel"),
        sha256 = "f5c93c146f52b4f1664fa3ce6579f961a910e869ab0dae431bd871bdd2584ef2",
        strip_prefix = "nasm-2.15.05",
        urls = [
            "https://mirror.bazel.build/www.nasm.us/pub/nasm/releasebuilds/2.15.05/win64/nasm-2.15.05-win64.zip",
            "https://www.nasm.us/pub/nasm/releasebuilds/2.15.05/win64/nasm-2.15.05-win64.zip",
        ],
    )

    maybe(
        http_archive,
        name = "rules_perl",
        strip_prefix = "rules_perl-0.2.5",
        urls = [
            "https://github.com/bazelbuild/rules_perl/archive/refs/tags/0.2.5.tar.gz",
        ],
    )
