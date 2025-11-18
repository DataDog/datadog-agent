"""A wrapper for defining stardoc dependencies to make instantiating
it in a WORKSPACE file more consistent
"""

load("@io_bazel_stardoc//:setup.bzl", _stardoc_repositories = "stardoc_repositories")

stardoc_deps = _stardoc_repositories
