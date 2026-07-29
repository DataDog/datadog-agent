"""Unit tests for rewrite_rpath."""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")
load("//bazel/rules/rewrite_rpath:rewrite_rpath.bzl", "relative_rpath")

_EMBEDDED = "/opt/datadog-agent/embedded"
_LIB = _EMBEDDED + "/lib"

def _regular_file(destination):
    return struct(
        target = struct(is_directory = False),
        destination = destination,
    )

def _tree_artifact(destination):
    return struct(
        target = struct(is_directory = True),
        destination = destination,
    )

def relative_rpath_regular_library_same_dir_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(
        env,
        "./",
        relative_rpath(_regular_file(_LIB + "/libpython3.13.dylib"), _LIB),
    )
    return unittest.end(env)

def relative_rpath_regular_binary_sibling_dir_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(
        env,
        "./../lib",
        relative_rpath(_regular_file(_EMBEDDED + "/bin/python3.13"), _LIB),
    )
    return unittest.end(env)

def relative_rpath_tree_root_parent_dir_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(
        env,
        "./..",
        relative_rpath(_tree_artifact(_LIB + "/python3.13"), _LIB),
    )
    return unittest.end(env)

def relative_rpath_tree_root_nested_parent_dir_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(
        env,
        "./../..",
        relative_rpath(_tree_artifact(_LIB + "/python3.13/lib-dynload"), _LIB),
    )
    return unittest.end(env)

def relative_rpath_regular_library_child_dir_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(
        env,
        "./msodbcsql18/lib64",
        relative_rpath(_regular_file(_LIB + "/libfoo.dylib"), _LIB + "/msodbcsql18/lib64"),
    )
    return unittest.end(env)

relative_rpath_regular_library_same_dir_test = unittest.make(relative_rpath_regular_library_same_dir_test_impl)
relative_rpath_regular_binary_sibling_dir_test = unittest.make(relative_rpath_regular_binary_sibling_dir_test_impl)
relative_rpath_tree_root_parent_dir_test = unittest.make(relative_rpath_tree_root_parent_dir_test_impl)
relative_rpath_tree_root_nested_parent_dir_test = unittest.make(relative_rpath_tree_root_nested_parent_dir_test_impl)
relative_rpath_regular_library_child_dir_test = unittest.make(relative_rpath_regular_library_child_dir_test_impl)

def rewrite_rpath_test_suite(name):
    unittest.suite(
        name,
        relative_rpath_regular_library_same_dir_test,
        relative_rpath_regular_binary_sibling_dir_test,
        relative_rpath_tree_root_parent_dir_test,
        relative_rpath_tree_root_nested_parent_dir_test,
        relative_rpath_regular_library_child_dir_test,
    )
