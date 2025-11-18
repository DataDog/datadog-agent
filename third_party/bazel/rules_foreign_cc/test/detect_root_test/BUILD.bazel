load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load(":detect_root_test_rule.bzl", "detect_root_test_rule")

filegroup(
    name = "fg",
    srcs = glob(
        ["dir1/**"],
    ),
)

filegroup(
    name = "fg_srcs",
    srcs = glob(
        ["dir1/srcs/**"],
    ),
)

detect_root_test_rule(
    name = "srcs_is_fg",
    srcs = ":fg",
    out = "out_fg.txt",
)

detect_root_test_rule(
    name = "srcs_is_fg_srcs",
    srcs = ":fg_srcs",
    out = "out_fg_srcs.txt",
)

detect_root_test_rule(
    name = "srcs_in_repo",
    srcs = "@rules_foreign_cc_detect_root_test_repo//:srcs",
    out = "out_repo.txt",
)

diff_test(
    name = "repo_test",
    file1 = ":srcs_in_repo",
    file2 = "expected/out_repo.txt",
)

diff_test(
    name = "fg_test",
    file1 = ":srcs_is_fg",
    file2 = "expected/out_fg.txt",
)

diff_test(
    name = "fg_srcs_test",
    file1 = ":srcs_is_fg_srcs",
    file2 = "expected/out_fg_srcs.txt",
)

test_suite(
    name = "tests",
    tests = [
        "fg_srcs_test",
        "fg_test",
        "repo_test",
    ],
)
