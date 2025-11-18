""" Provides linting aspects for shellcheck used to check validity of generated shell scripts. """

load("@aspect_rules_lint//lint:lint_test.bzl", "lint_test")
load("@aspect_rules_lint//lint:shellcheck.bzl", "lint_shellcheck_aspect")

shellcheck = lint_shellcheck_aspect(
    binary = "@multitool//tools/shellcheck",
    config = Label("@//:.shellcheckrc"),
)

shellcheck_test = lint_test(aspect = shellcheck)
