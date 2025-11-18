""" Rule for building Boost from sources. """

load("@rules_cc//cc:defs.bzl", "CcInfo")
load("//foreign_cc/private:detect_root.bzl", "detect_root")
load(
    "//foreign_cc/private:framework.bzl",
    "CC_EXTERNAL_RULE_ATTRIBUTES",
    "CC_EXTERNAL_RULE_FRAGMENTS",
    "cc_external_rule_impl",
    "create_attrs",
    "expand_locations_and_make_variables",
)

def _boost_build_impl(ctx):
    attrs = create_attrs(
        ctx.attr,
        configure_name = "BoostBuild",
        create_configure_script = _create_configure_script,
        tools_data = [],
    )
    return cc_external_rule_impl(ctx, attrs)

def _create_configure_script(configureParameters):
    ctx = configureParameters.ctx
    root = detect_root(ctx.attr.lib_source)
    data = ctx.attr.data + ctx.attr.build_data
    user_options = expand_locations_and_make_variables(ctx, ctx.attr.user_options, "user_options", data)

    return [
        "cd $INSTALLDIR",
        "##copy_dir_contents_to_dir## $$EXT_BUILD_ROOT$$/{}/. .".format(root),
        "chmod -R +w .",
        "##enable_tracing##",
        "./bootstrap.sh {}".format(" ".join(ctx.attr.bootstrap_options)),
        "./b2 install {} --prefix=.".format(" ".join(user_options)),
        "##disable_tracing##",
    ]

def _attrs():
    attrs = dict(CC_EXTERNAL_RULE_ATTRIBUTES)
    attrs.pop("targets")
    attrs.update({
        "bootstrap_options": attr.string_list(
            doc = "any additional flags to pass to bootstrap.sh",
            mandatory = False,
        ),
        "user_options": attr.string_list(
            doc = "any additional flags to pass to b2",
            mandatory = False,
        ),
    })
    return attrs

boost_build = rule(
    doc = "Rule for building Boost. Invokes bootstrap.sh and then b2 install.",
    attrs = _attrs(),
    fragments = CC_EXTERNAL_RULE_FRAGMENTS,
    output_to_genfiles = True,
    provides = [CcInfo],
    implementation = _boost_build_impl,
    toolchains = [
        "@rules_foreign_cc//foreign_cc/private/framework:shell_toolchain",
        "@bazel_tools//tools/cpp:toolchain_type",
    ],
)
