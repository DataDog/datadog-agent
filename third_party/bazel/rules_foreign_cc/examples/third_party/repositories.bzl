"""A centralized module defining all repositories required for third party examples of rules_foreign_cc"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")
load("//apr:apr_repositories.bzl", "apr_repositories")
load("//apr_util:apr_util_repositories.bzl", "apr_util_repositories")
load("//autotools:autotools_repositories.bzl", "autotools_repositories")
load("//bison:bison_repositories.bzl", "bison_repositories")
load("//cares:cares_repositories.bzl", "cares_repositories")
load("//curl:curl_repositories.bzl", "curl_repositories")
load("//glib:glib_repositories.bzl", "glib_repositories")
load("//gn:gn_repositories.bzl", "gn_repositories")
load("//gperftools:gperftools_repositories.bzl", "gperftools_repositories")
load("//iconv:iconv_repositories.bzl", "iconv_repositories")
load("//libgit2:libgit2_repositories.bzl", "libgit2_repositories")
load("//libjpeg_turbo:libjpeg_turbo_repositories.bzl", "libjpeg_turbo_repositories")
load("//libpng:libpng_repositories.bzl", "libpng_repositories")
load("//libssh2:libssh2_repositories.bzl", "libssh2_repositories")
load("//log4cxx:log4cxx_repositories.bzl", "log4cxx_repositories")
load("//mesa:mesa_repositories.bzl", "mesa_repositories")
load("//openssl:openssl_repositories.bzl", "openssl_repositories")
load("//pcre:pcre_repositories.bzl", "pcre_repositories")
load("//python:python_repositories.bzl", "python_repositories")
load("//sqlite:sqlite_repositories.bzl", "sqlite_repositories")
load("//subversion:subversion_repositories.bzl", "subversion_repositories")
load("//zlib:zlib_repositories.bzl", "zlib_repositories")

# buildifier: disable=unnamed-macro
def repositories():
    """Load all repositories needed for the targets of rules_foreign_cc_examples_third_party"""
    apr_repositories()
    apr_util_repositories()
    autotools_repositories()
    bison_repositories()
    cares_repositories()
    curl_repositories()
    glib_repositories()
    gn_repositories()
    gperftools_repositories()
    iconv_repositories()
    libgit2_repositories()
    libjpeg_turbo_repositories()
    libpng_repositories()
    libssh2_repositories()
    log4cxx_repositories()
    mesa_repositories()
    openssl_repositories()
    pcre_repositories()
    python_repositories()
    sqlite_repositories()
    subversion_repositories()
    zlib_repositories()

    maybe(
        http_archive,
        name = "rules_cc",
        urls = ["https://github.com/bazelbuild/rules_cc/releases/download/0.0.1/rules_cc-0.0.1.tar.gz"],
        sha256 = "4dccbfd22c0def164c8f47458bd50e0c7148f3d92002cdb459c2a96a68498241",
    )
