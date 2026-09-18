"""Tests for check_for_symbols.bzl - the check_for_symbols_test rule."""

load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")
load("@rules_testing//lib:truth.bzl", "matching")
load("@rules_testing//lib:util.bzl", "util")
load(":check_for_symbols.bzl", "check_for_symbols")

def _test_wires_checker_action(name):
    util.helper_target(
        check_for_symbols,
        name = name + "_subject",
        binary = "//bazel/rules/check_for_symbols:test_binary",
        must_include = ["wanted_symbol"],
        must_not_include = ["unwanted_symbol"],
        testonly = True,
    )
    analysis_test(
        name = name,
        impl = _test_wires_checker_action_impl,
        target = name + "_subject",
    )

def _test_wires_checker_action_impl(env, target):
    subject = env.expect.that_target(target)
    out_path = target.label.package + "/" + target.label.name + ".status"
    subject.default_outputs().contains(out_path)

    action = subject.actual.actions[0]
    env.expect.that_str(action.mnemonic).equals("CheckForSymbols")

    argv = action.argv
    env.expect.that_collection(argv).contains_at_least([
        "--must-include",
        "wanted_symbol",
        "--must-not-include",
        "unwanted_symbol",
    ])

def _test_requires_at_least_one_pattern(name):
    util.helper_target(
        check_for_symbols,
        name = name + "_subject",
        binary = "//bazel/rules/check_for_symbols:test_binary",
        must_include = [],
        must_not_include = [],
        testonly = True,
    )
    analysis_test(
        name = name,
        impl = _test_requires_at_least_one_pattern_impl,
        target = name + "_subject",
        expect_failure = True,
    )

def _test_requires_at_least_one_pattern_impl(env, target):
    env.expect.that_target(target).failures().contains_predicate(
        matching.str_matches("*must be non-empty*"),
    )

def check_for_symbols_test_suite(name):
    test_suite(
        name = name,
        tests = [
            _test_wires_checker_action,
            _test_requires_at_least_one_pattern,
        ],
    )
