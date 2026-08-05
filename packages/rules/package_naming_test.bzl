"""Tests for package_naming.bzl - package naming variables rule."""

load("@rules_pkg//pkg:providers.bzl", "PackageVariablesInfo")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:util.bzl", "util")
load(":package_naming.bzl", "package_name_variables")

# -- Transitions for flavor testing ------------------------------------------
# Each transition forces //packages/agent:flavor to a specific value so we can
# verify _inject_flavor() output without changing the default flag in the repo.

def _fips_transition_impl(_settings, _attr):
    return {"//packages/agent:flavor": "fips"}

_fips_transition = transition(
    implementation = _fips_transition_impl,
    inputs = [],
    outputs = ["//packages/agent:flavor"],
)

def _heroku_transition_impl(_settings, _attr):
    return {"//packages/agent:flavor": "heroku"}

_heroku_transition = transition(
    implementation = _heroku_transition_impl,
    inputs = [],
    outputs = ["//packages/agent:flavor"],
)

def _flavored_vars_impl(ctx):
    # With a Starlark cfg transition on a label attr, ctx.attr.inner is a list
    # (one entry per transition output).  Our transitions are 1:1, so take [0].
    return [ctx.attr.inner[0][PackageVariablesInfo]]

# Wrapper rules that apply the transitions to the inner package_name_variables.
_fips_vars = rule(
    implementation = _flavored_vars_impl,
    attrs = {
        "inner": attr.label(cfg = _fips_transition, providers = [PackageVariablesInfo]),
    },
)

_heroku_vars = rule(
    implementation = _flavored_vars_impl,
    attrs = {
        "inner": attr.label(cfg = _heroku_transition, providers = [PackageVariablesInfo]),
    },
)

# -- Test 1: All expected keys are present in the provider -------------------

def _test_provider_keys(name):
    util.helper_target(
        package_name_variables,
        name = name + "_subject",
    )
    analysis_test(
        name = name,
        impl = _test_provider_keys_impl,
        target = name + "_subject",
    )

def _test_provider_keys_impl(env, target):
    pvi = target[PackageVariablesInfo]
    env.expect.that_collection(pvi.values.keys()).contains_at_least([
        "product_name",
        "version",
        "arch_deb",
        "arch_rpm",
        "cpu",
        "compiler",
        "libc",
        "base_branch",
        "milestone",
        "compilation_mode",
    ])

# -- Test 2: Default (base) flavor leaves product_name unchanged -------------

def _test_base_flavor_product_name(name):
    util.helper_target(
        package_name_variables,
        name = name + "_subject",
        product_name = "datadog-agent",
    )
    analysis_test(
        name = name,
        impl = _test_base_flavor_product_name_impl,
        target = name + "_subject",
    )

def _test_base_flavor_product_name_impl(env, target):
    pvi = target[PackageVariablesInfo]
    env.expect.that_str(pvi.values.get("product_name")).equals("datadog-agent")

# -- Test 3: FIPS flavor inserts "fips" after the first word -----------------
# "datadog-agent" => "datadog-fips-agent"

def _test_fips_flavor_product_name(name):
    util.helper_target(
        package_name_variables,
        name = name + "_inner",
        product_name = "datadog-agent",
    )
    util.helper_target(
        _fips_vars,
        name = name + "_subject",
        inner = name + "_inner",
    )
    analysis_test(
        name = name,
        impl = _test_fips_flavor_product_name_impl,
        target = name + "_subject",
    )

def _test_fips_flavor_product_name_impl(env, target):
    pvi = target[PackageVariablesInfo]
    env.expect.that_str(pvi.values.get("product_name")).equals("datadog-fips-agent")

# -- Test 4: FIPS + three-word product name ----------------------------------
# "datadog-agent-dbg" => "datadog-fips-agent-dbg"
# Exercises the words[0] + flavor + "-".join(words[1:]) branch.

def _test_fips_flavor_multiword(name):
    util.helper_target(
        package_name_variables,
        name = name + "_inner",
        product_name = "datadog-agent-dbg",
    )
    util.helper_target(
        _fips_vars,
        name = name + "_subject",
        inner = name + "_inner",
    )
    analysis_test(
        name = name,
        impl = _test_fips_flavor_multiword_impl,
        target = name + "_subject",
    )

def _test_fips_flavor_multiword_impl(env, target):
    pvi = target[PackageVariablesInfo]
    env.expect.that_str(pvi.values.get("product_name")).equals("datadog-fips-agent-dbg")

# ── Test 5: FIPS + single-word product name ──────────────────────────────────
# A product name with no dash gets the flavor appended: "agent" => "agent-fips"

def _test_fips_flavor_single_word(name):
    util.helper_target(
        package_name_variables,
        name = name + "_inner",
        product_name = "agent",
    )
    util.helper_target(
        _fips_vars,
        name = name + "_subject",
        inner = name + "_inner",
    )
    analysis_test(
        name = name,
        impl = _test_fips_flavor_single_word_impl,
        target = name + "_subject",
    )

def _test_fips_flavor_single_word_impl(env, target):
    pvi = target[PackageVariablesInfo]
    env.expect.that_str(pvi.values.get("product_name")).equals("agent-fips")

# -- Test 6: Heroku flavor ---------------------------------------------------
# "datadog-agent" => "datadog-heroku-agent"

def _test_heroku_flavor_product_name(name):
    util.helper_target(
        package_name_variables,
        name = name + "_inner",
        product_name = "datadog-agent",
    )
    util.helper_target(
        _heroku_vars,
        name = name + "_subject",
        inner = name + "_inner",
    )
    analysis_test(
        name = name,
        impl = _test_heroku_flavor_product_name_impl,
        target = name + "_subject",
    )

def _test_heroku_flavor_product_name_impl(env, target):
    pvi = target[PackageVariablesInfo]
    env.expect.that_str(pvi.values.get("product_name")).equals("datadog-heroku-agent")

# -- Test 7: arch_deb and arch_rpm map correctly for the current platform ----
# x86_64 / k8  => deb: "amd64",  rpm: "x86_64"
# aarch64 / arm => deb: "arm64",  rpm: "arm64"
# We express the expected value via select() so the test is correct on every
# build host without hard-coding a single architecture.

def _test_arch_values(name):
    util.helper_target(
        package_name_variables,
        name = name + "_subject",
    )
    analysis_test(
        name = name,
        impl = _test_arch_values_impl,
        target = name + "_subject",
        attr_values = {
            "expect_arch_deb": select({
                "@platforms//cpu:arm64": "arm64",
                "@platforms//cpu:x86_64": "amd64",
                "//conditions:default": "amd64",
            }),
            "expect_arch_rpm": select({
                "@platforms//cpu:arm64": "aarch64",
                "@platforms//cpu:x86_64": "x86_64",
                "//conditions:default": "x86_64",
            }),
        },
        attrs = {
            "expect_arch_deb": attr.string(),
            "expect_arch_rpm": attr.string(),
        },
    )

def _test_arch_values_impl(env, target):
    pvi = target[PackageVariablesInfo]
    env.expect.that_str(pvi.values.get("arch_deb")).equals(env.ctx.attr.expect_arch_deb)
    env.expect.that_str(pvi.values.get("arch_rpm")).equals(env.ctx.attr.expect_arch_rpm)

# -- Test 8: version begins with "7" and contains "-localbuild" --------------
# The exact value depends on CI env vars / release.json, but without
# PACKAGE_VERSION set the fallback path produces "<milestone>-localbuild"
# where milestone is a "7.x.y" string from release.json.

def _test_version_nonempty(name):
    util.helper_target(
        package_name_variables,
        name = name + "_subject",
    )
    analysis_test(
        name = name,
        impl = _test_version_nonempty_impl,
        target = name + "_subject",
    )

def _test_version_nonempty_impl(env, target):
    pvi = target[PackageVariablesInfo]
    version = pvi.values.get("version")
    env.expect.that_str(version[:2]).equals("7.")
    env.expect.that_str(version).contains("-localbuild")

# -- Suite -------------------------------------------------------------------

def package_naming_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_provider_keys,
            _test_base_flavor_product_name,
            _test_fips_flavor_product_name,
            _test_fips_flavor_multiword,
            _test_fips_flavor_single_word,
            _test_heroku_flavor_product_name,
            _test_arch_values,
            _test_version_nonempty,
        ],
    )
