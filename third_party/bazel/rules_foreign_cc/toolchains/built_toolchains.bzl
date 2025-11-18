"""
Defines repositories and register toolchains for versions of the tools built
from source
"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")
load("@rules_foreign_cc//toolchains:cmake_versions.bzl", _CMAKE_SRCS = "CMAKE_SRCS")

_ALL_CONTENT = """\
filegroup(
    name = "all_srcs",
    srcs = glob(["**"]),
    visibility = ["//visibility:public"],
)
"""

_MESON_BUILD_FILE_CONTENT = """\
exports_files(["meson.py"])

filegroup(
    name = "runtime",
    # NOTE: excluding __pycache__ is important to avoid rebuilding due to pyc
    # files, see https://github.com/bazel-contrib/rules_foreign_cc/issues/1342
    srcs = glob(["mesonbuild/**"], exclude = ["**/__pycache__/*"]),
    visibility = ["//visibility:public"],
)
"""

# buildifier: disable=unnamed-macro
def built_toolchains(cmake_version, make_version, ninja_version, meson_version, pkgconfig_version, register_toolchains, register_built_pkgconfig_toolchain):
    """
    Register toolchains for built tools that will be built from source


    Args:
        cmake_version: The CMake version to build
        make_version: The Make version to build
        ninja_version: The Ninja version to build
        meson_version: The Meson version to build
        pkgconfig_version: The pkg-config version to build
        register_toolchains: If true, registers the toolchains via native.register_toolchains. Used by bzlmod
        register_built_pkgconfig_toolchain: If true, the built pkgconfig toolchain will be registered.
    """
    _cmake_toolchain(cmake_version, register_toolchains)
    _make_toolchain(make_version, register_toolchains)
    _ninja_toolchain(ninja_version, register_toolchains)
    _meson_toolchain(meson_version, register_toolchains)

    if register_built_pkgconfig_toolchain:
        _pkgconfig_toolchain(pkgconfig_version, register_toolchains)

def _cmake_toolchain(version, register_toolchains):
    if register_toolchains:
        native.register_toolchains(
            "@rules_foreign_cc//toolchains:built_cmake_toolchain",
        )

    if _CMAKE_SRCS.get(version):
        cmake_meta = _CMAKE_SRCS[version]
        urls = cmake_meta[0]
        prefix = cmake_meta[1]
        sha256 = cmake_meta[2]
        maybe(
            http_archive,
            name = "cmake_src",
            build_file_content = _ALL_CONTENT,
            sha256 = sha256,
            strip_prefix = prefix,
            urls = urls,
            patches = [
                Label("//toolchains/patches:cmake-c++11.patch"),
            ],
        )
        return

    fail("Unsupported cmake version: " + str(version))

def _make_toolchain(version, register_toolchains):
    if register_toolchains:
        native.register_toolchains(
            "@rules_foreign_cc//toolchains:built_make_toolchain",
        )

    if version == "4.4.1":
        maybe(
            http_archive,
            name = "gnumake_src",
            build_file_content = _ALL_CONTENT,
            sha256 = "dd16fb1d67bfab79a72f5e8390735c49e3e8e70b4945a15ab1f81ddb78658fb3",
            strip_prefix = "make-4.4.1",
            urls = [
                "https://mirror.bazel.build/ftpmirror.gnu.org/gnu/make/make-4.4.1.tar.gz",
                "http://ftpmirror.gnu.org/gnu/make/make-4.4.1.tar.gz",
            ],
        )
        return

    if version == "4.4":
        maybe(
            http_archive,
            name = "gnumake_src",
            build_file_content = _ALL_CONTENT,
            sha256 = "581f4d4e872da74b3941c874215898a7d35802f03732bdccee1d4a7979105d18",
            strip_prefix = "make-4.4",
            urls = [
                "https://mirror.bazel.build/ftpmirror.gnu.org/gnu/make/make-4.4.tar.gz",
                "http://ftpmirror.gnu.org/gnu/make/make-4.4.tar.gz",
            ],
        )
        return
    if version == "4.3":
        maybe(
            http_archive,
            name = "gnumake_src",
            build_file_content = _ALL_CONTENT,
            patches = [Label("//toolchains/patches:make-reproducible-bootstrap.patch")],
            sha256 = "e05fdde47c5f7ca45cb697e973894ff4f5d79e13b750ed57d7b66d8defc78e19",
            strip_prefix = "make-4.3",
            urls = [
                "https://mirror.bazel.build/ftpmirror.gnu.org/gnu/make/make-4.3.tar.gz",
                "http://ftpmirror.gnu.org/gnu/make/make-4.3.tar.gz",
            ],
        )
        return

    fail("Unsupported make version: " + str(version))

def _ninja_toolchain(version, register_toolchains):
    if register_toolchains:
        native.register_toolchains(
            "@rules_foreign_cc//toolchains:built_ninja_toolchain",
        )
    if version == "1.13.0":
        maybe(
            http_archive,
            name = "ninja_build_src",
            build_file_content = _ALL_CONTENT,
            integrity = "sha256-8IZB0ACZqeQNROwBRvhBxHKuWLfm3VF77jlFz9kjzt8=",
            strip_prefix = "ninja-1.13.0",
            urls = [
                "https://mirror.bazel.build/github.com/ninja-build/ninja/archive/v1.13.0.tar.gz",
                "https://github.com/ninja-build/ninja/archive/v1.13.0.tar.gz",
            ],
        )
        return
    if version == "1.12.1":
        maybe(
            http_archive,
            name = "ninja_build_src",
            build_file_content = _ALL_CONTENT,
            integrity = "sha256-ghvf9Io/aDvEuztvC1/nstZHz2XVKutjMoyRpsbfKFo=",
            strip_prefix = "ninja-1.12.1",
            urls = [
                "https://mirror.bazel.build/github.com/ninja-build/ninja/archive/v1.12.1.tar.gz",
                "https://github.com/ninja-build/ninja/archive/v1.12.1.tar.gz",
            ],
        )
        return
    if version == "1.12.0":
        maybe(
            http_archive,
            name = "ninja_build_src",
            build_file_content = _ALL_CONTENT,
            integrity = "sha256-iyyGzUg9x/y3l1xexzKRNdIQCZqJvH2wWQoHsLv+SaU=",
            strip_prefix = "ninja-1.12.0",
            urls = [
                "https://mirror.bazel.build/github.com/ninja-build/ninja/archive/v1.12.0.tar.gz",
                "https://github.com/ninja-build/ninja/archive/v1.12.0.tar.gz",
            ],
        )
        return
    if version == "1.11.1":
        maybe(
            http_archive,
            name = "ninja_build_src",
            build_file_content = _ALL_CONTENT,
            sha256 = "31747ae633213f1eda3842686f83c2aa1412e0f5691d1c14dbbcc67fe7400cea",
            strip_prefix = "ninja-1.11.1",
            urls = [
                "https://mirror.bazel.build/github.com/ninja-build/ninja/archive/v1.11.1.tar.gz",
                "https://github.com/ninja-build/ninja/archive/v1.11.1.tar.gz",
            ],
        )
        return
    if version == "1.11.0":
        maybe(
            http_archive,
            name = "ninja_build_src",
            build_file_content = _ALL_CONTENT,
            sha256 = "3c6ba2e66400fe3f1ae83deb4b235faf3137ec20bd5b08c29bfc368db143e4c6",
            strip_prefix = "ninja-1.11.0",
            urls = [
                "https://mirror.bazel.build/github.com/ninja-build/ninja/archive/v1.11.0.tar.gz",
                "https://github.com/ninja-build/ninja/archive/v1.11.0.tar.gz",
            ],
        )
        return
    if version == "1.10.2":
        maybe(
            http_archive,
            name = "ninja_build_src",
            build_file_content = _ALL_CONTENT,
            sha256 = "ce35865411f0490368a8fc383f29071de6690cbadc27704734978221f25e2bed",
            strip_prefix = "ninja-1.10.2",
            urls = [
                "https://mirror.bazel.build/github.com/ninja-build/ninja/archive/v1.10.2.tar.gz",
                "https://github.com/ninja-build/ninja/archive/v1.10.2.tar.gz",
            ],
        )
        return

    fail("Unsupported ninja version: " + str(version))

def _meson_toolchain(version, register_toolchains):
    if register_toolchains:
        native.register_toolchains(
            "@rules_foreign_cc//toolchains:built_meson_toolchain",
        )
    if version == "1.5.1":
        maybe(
            http_archive,
            name = "meson_src",
            build_file_content = _MESON_BUILD_FILE_CONTENT,
            sha256 = "567e533adf255de73a2de35049b99923caf872a455af9ce03e01077e0d384bed",
            strip_prefix = "meson-1.5.1",
            urls = [
                "https://mirror.bazel.build/github.com/mesonbuild/meson/releases/download/1.5.1/meson-1.5.1.tar.gz",
                "https://github.com/mesonbuild/meson/releases/download/1.5.1/meson-1.5.1.tar.gz",
            ],
        )
        return
    if version == "1.1.1":
        maybe(
            http_archive,
            name = "meson_src",
            build_file_content = _MESON_BUILD_FILE_CONTENT,
            sha256 = "d04b541f97ca439fb82fab7d0d480988be4bd4e62563a5ca35fadb5400727b1c",
            strip_prefix = "meson-1.1.1",
            urls = [
                "https://mirror.bazel.build/github.com/mesonbuild/meson/releases/download/1.1.1/meson-1.1.1.tar.gz",
                "https://github.com/mesonbuild/meson/releases/download/1.1.1/meson-1.1.1.tar.gz",
            ],
        )
        return
    if version == "0.63.0":
        maybe(
            http_archive,
            name = "meson_src",
            build_file_content = _MESON_BUILD_FILE_CONTENT,
            sha256 = "3b51d451744c2bc71838524ec8d96cd4f8c4793d5b8d5d0d0a9c8a4f7c94cd6f",
            strip_prefix = "meson-0.63.0",
            urls = [
                "https://mirror.bazel.build/github.com/mesonbuild/meson/releases/download/0.63.0/meson-0.63.0.tar.gz",
                "https://github.com/mesonbuild/meson/releases/download/0.63.0/meson-0.63.0.tar.gz",
            ],
        )
        return

    fail("Unsupported meson version: " + str(version))

def _pkgconfig_toolchain(version, register_toolchains):
    if register_toolchains:
        native.register_toolchains(
            "@rules_foreign_cc//toolchains:built_pkgconfig_toolchain",
        )

    maybe(
        http_archive,
        name = "glib_dev",
        build_file_content = '''
cc_import(
    name = "glib_dev",
    hdrs = glob(["include/**"]),
    shared_library = "@glib_runtime//:bin/libglib-2.0-0.dll",
    visibility = ["//visibility:public"],
)
        ''',
        sha256 = "bdf18506df304d38be98a4b3f18055b8b8cca81beabecad0eece6ce95319c369",
        urls = [
            "https://mirror.bazel.build/download.gnome.org/binaries/win64/glib/2.26/glib-dev_2.26.1-1_win64.zip",
            "https://download.gnome.org/binaries/win64/glib/2.26/glib-dev_2.26.1-1_win64.zip",
        ],
    )

    maybe(
        http_archive,
        name = "glib_src",
        build_file_content = '''
cc_import(
    name = "msvc_hdr",
    hdrs = ["msvc_recommended_pragmas.h"],
    visibility = ["//visibility:public"],
)
        ''',
        sha256 = "bc96f63112823b7d6c9f06572d2ad626ddac7eb452c04d762592197f6e07898e",
        strip_prefix = "glib-2.26.1",
        urls = [
            "https://mirror.bazel.build/download.gnome.org/sources/glib/2.26/glib-2.26.1.tar.gz",
            "https://download.gnome.org/sources/glib/2.26/glib-2.26.1.tar.gz",
        ],
    )

    maybe(
        http_archive,
        name = "glib_runtime",
        build_file_content = '''
exports_files(
    [
        "bin/libgio-2.0-0.dll",
        "bin/libglib-2.0-0.dll",
        "bin/libgmodule-2.0-0.dll",
        "bin/libgobject-2.0-0.dll",
        "bin/libgthread-2.0-0.dll",
    ],
    visibility = ["//visibility:public"],
)
        ''',
        sha256 = "88d857087e86f16a9be651ee7021880b3f7ba050d34a1ed9f06113b8799cb973",
        urls = [
            "https://mirror.bazel.build/download.gnome.org/binaries/win64/glib/2.26/glib_2.26.1-1_win64.zip",
            "https://download.gnome.org/binaries/win64/glib/2.26/glib_2.26.1-1_win64.zip",
        ],
    )

    maybe(
        http_archive,
        name = "gettext_runtime",
        build_file_content = '''
cc_import(
    name = "gettext_runtime",
    shared_library = "bin/libintl-8.dll",
    visibility = ["//visibility:public"],
)
        ''',
        sha256 = "1f4269c0e021076d60a54e98da6f978a3195013f6de21674ba0edbc339c5b079",
        urls = [
            "https://mirror.bazel.build/download.gnome.org/binaries/win64/dependencies/gettext-runtime_0.18.1.1-2_win64.zip",
            "https://download.gnome.org/binaries/win64/dependencies/gettext-runtime_0.18.1.1-2_win64.zip",
        ],
    )
    if version == "0.29.2":
        maybe(
            http_archive,
            name = "pkgconfig_src",
            build_file_content = _ALL_CONTENT,
            sha256 = "6fc69c01688c9458a57eb9a1664c9aba372ccda420a02bf4429fe610e7e7d591",
            strip_prefix = "pkg-config-0.29.2",
            # The patch is required as bazel does not provide the VCINSTALLDIR or WINDOWSSDKDIR vars
            patches = [
                # This patch is required as bazel does not provide the VCINSTALLDIR or WINDOWSSDKDIR vars
                Label("//toolchains/patches:pkgconfig-detectenv.patch"),

                # This patch is required as rules_foreign_cc runs in MSYS2 on Windows and MSYS2's "mkdir" is used
                Label("//toolchains/patches:pkgconfig-makefile-vc.patch"),

                # This patch fixes explicit integer conversion which causes errors in clang >= 15 and gcc >= 14
                Label("//toolchains/patches:pkgconfig-builtin-glib-int-conversion.patch"),
            ],
            urls = [
                "https://mirror.bazel.build/pkgconfig.freedesktop.org/releases/pkg-config-0.29.2.tar.gz",
                "https://pkgconfig.freedesktop.org/releases/pkg-config-0.29.2.tar.gz",
            ],
        )
        return

    fail("Unsupported pkgconfig version: " + str(version))
