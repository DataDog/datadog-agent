"""A module defining the various toolchain definitions for `rules_foreign_cc`"""

load(":built_toolchains.bzl", _built_toolchains = "built_toolchains")
load(":prebuilt_toolchains.bzl", _prebuilt_toolchains = "prebuilt_toolchains")

# Re-expose the built toolchains macro
built_toolchains = _built_toolchains

# Re-expose the prebuilt toolchains macro
prebuilt_toolchains = _prebuilt_toolchains

# buildifier: disable=unnamed-macro
def preinstalled_toolchains():
    """Register toolchains for various build tools expected to be installed on the exec host"""
    native.register_toolchains(
        "@rules_foreign_cc//toolchains:preinstalled_cmake_toolchain",
        "@rules_foreign_cc//toolchains:preinstalled_make_toolchain",
        "@rules_foreign_cc//toolchains:preinstalled_ninja_toolchain",
        "@rules_foreign_cc//toolchains:preinstalled_meson_toolchain",
        "@rules_foreign_cc//toolchains:preinstalled_autoconf_toolchain",
        "@rules_foreign_cc//toolchains:preinstalled_automake_toolchain",
        "@rules_foreign_cc//toolchains:preinstalled_m4_toolchain",
        "@rules_foreign_cc//toolchains:preinstalled_pkgconfig_toolchain",
    )

def _current_toolchain_impl(ctx):
    toolchain = ctx.toolchains[ctx.attr._toolchain]

    if toolchain.data.target:
        return [
            toolchain,
            platform_common.TemplateVariableInfo(toolchain.data.env),
            DefaultInfo(
                files = toolchain.data.target.files,
                runfiles = toolchain.data.target.default_runfiles,
            ),
        ]
    return [
        toolchain,
        platform_common.TemplateVariableInfo(toolchain.data.env),
        DefaultInfo(),
    ]

# These rules exist so that the current toolchain can be used in the `toolchains` attribute of
# other rules, such as genrule. It allows exposing a <tool>_toolchain after toolchain resolution has
# happened, to a rule which expects a concrete implementation of a toolchain, rather than a
# toochain_type which could be resolved to that toolchain.
#
# See https://github.com/bazelbuild/bazel/issues/14009#issuecomment-921960766
current_cmake_toolchain = rule(
    implementation = _current_toolchain_impl,
    attrs = {
        "_toolchain": attr.string(default = str(Label("//toolchains:cmake_toolchain"))),
    },
    toolchains = [
        str(Label("//toolchains:cmake_toolchain")),
    ],
)

current_make_toolchain = rule(
    implementation = _current_toolchain_impl,
    attrs = {
        "_toolchain": attr.string(default = str(Label("//toolchains:make_toolchain"))),
    },
    toolchains = [
        str(Label("//toolchains:make_toolchain")),
    ],
)

current_ninja_toolchain = rule(
    implementation = _current_toolchain_impl,
    attrs = {
        "_toolchain": attr.string(default = str(Label("//toolchains:ninja_toolchain"))),
    },
    toolchains = [
        str(Label("//toolchains:ninja_toolchain")),
    ],
)

current_meson_toolchain = rule(
    implementation = _current_toolchain_impl,
    attrs = {
        "_toolchain": attr.string(default = str(Label("//toolchains:meson_toolchain"))),
    },
    toolchains = [
        str(Label("//toolchains:meson_toolchain")),
    ],
)

current_autoconf_toolchain = rule(
    implementation = _current_toolchain_impl,
    attrs = {
        "_toolchain": attr.string(default = str(Label("//toolchains:autoconf_toolchain"))),
    },
    toolchains = [
        str(Label("//toolchains:autoconf_toolchain")),
    ],
)

current_automake_toolchain = rule(
    implementation = _current_toolchain_impl,
    attrs = {
        "_toolchain": attr.string(default = str(Label("//toolchains:automake_toolchain"))),
    },
    toolchains = [
        str(Label("//toolchains:automake_toolchain")),
    ],
)

current_m4_toolchain = rule(
    implementation = _current_toolchain_impl,
    attrs = {
        "_toolchain": attr.string(default = str(Label("//toolchains:m4_toolchain"))),
    },
    toolchains = [
        str(Label("//toolchains:m4_toolchain")),
    ],
)

current_pkgconfig_toolchain = rule(
    implementation = _current_toolchain_impl,
    attrs = {
        "_toolchain": attr.string(default = str(Label("//toolchains:pkgconfig_toolchain"))),
    },
    toolchains = [
        str(Label("//toolchains:pkgconfig_toolchain")),
    ],
)
