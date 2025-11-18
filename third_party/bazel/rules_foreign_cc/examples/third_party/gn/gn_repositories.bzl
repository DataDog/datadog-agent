"""A module defining the third party dependency gn"""

load("@bazel_tools//tools/build_defs/repo:git.bzl", "new_git_repository")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def gn_repositories():
    maybe(
        new_git_repository,
        name = "gn",
        build_file = Label("//gn:BUILD.gn.bazel"),
        commit = "dfcbc6fed0a8352696f92d67ccad54048ad182b3",
        patch_args = [],
        patch_tool = "bash",
        patches = [Label("//gn:patch.gen_ninja.sh")],
        remote = "https://gn.googlesource.com/gn",
        shallow_since = "1612864120 +0000",
    )
