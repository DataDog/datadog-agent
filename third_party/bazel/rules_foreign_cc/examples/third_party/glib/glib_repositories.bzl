"""A module defining the third party dependency glib"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def glib_repositories():
    maybe(
        http_archive,
        name = "glib",
        build_file = Label("//glib:BUILD.glib.bazel"),
        strip_prefix = "glib-2.77.0",
        sha256 = "1897fd8ad4ebb523c32fabe7508c3b0b039c089661ae1e7917df0956a320ac4d",
        url = "https://download.gnome.org/sources/glib/2.77/glib-2.77.0.tar.xz",
    )
    maybe(
        http_archive,
        name = "libffi",
        build_file = Label("//glib:BUILD.libffi.bazel"),
        strip_prefix = "libffi-meson-3.2.9999.3",
        sha256 = "0113d0f27ffe795158d06f56c9a7340fafc768586095b82a701c687ecb8e3672",
        url = "https://gitlab.freedesktop.org/gstreamer/meson-ports/libffi/-/archive/meson-3.2.9999.3/libffi-meson-3.2.9999.3.tar.gz",
    )
    maybe(
        http_archive,
        name = "gettext",
        build_file = Label("//glib:BUILD.gettext.bazel"),
        strip_prefix = "gettext-0.21.1",
        sha256 = "e8c3650e1d8cee875c4f355642382c1df83058bd5a11ee8555c0cf276d646d45",
        url = "https://ftp.gnu.org/gnu/gettext/gettext-0.21.1.tar.gz",
    )
    maybe(
        http_archive,
        name = "gettext_win",
        build_file = Label("//glib:BUILD.gettext_win.bazel"),
        sha256 = "0af0a6e2c26dd2c389b4cd5a473e121dad6ddf2f8dca38489c50858c7b8cdd9f",
        url = "https://download.gnome.org/binaries/win64/dependencies/gettext-runtime-dev_0.18.1.1-2_win64.zip",
    )
