"""Unit tests for cgo_godefs helpers."""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")
load("//bazel/rules/ebpf:cgo_godefs.bzl", "INTERNAL_FOR_TESTING")

_relpath = INTERNAL_FOR_TESTING["relpath"]

def _relpath_sibling_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, "../bar", _relpath("pkg/bar", "pkg/foo"))
    return unittest.end(env)

def _relpath_nested_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, "../../../other/bar", _relpath("other/bar", "pkg/sub/foo"))
    return unittest.end(env)

def _relpath_same_dir_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, "file.h", _relpath("pkg/foo/file.h", "pkg/foo"))
    return unittest.end(env)

def _relpath_empty_target_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, "../..", _relpath("", "pkg/foo"))
    return unittest.end(env)

def _relpath_empty_base_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, "pkg/foo", _relpath("pkg/foo", ""))
    return unittest.end(env)

def _relpath_identical_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, ".", _relpath("pkg/foo", "pkg/foo"))
    return unittest.end(env)

def _relpath_absolute_test_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, "../../usr/include", _relpath("/usr/include", "/home/user"))
    return unittest.end(env)

_relpath_sibling_test = unittest.make(_relpath_sibling_test_impl)
_relpath_nested_test = unittest.make(_relpath_nested_test_impl)
_relpath_same_dir_test = unittest.make(_relpath_same_dir_test_impl)
_relpath_empty_target_test = unittest.make(_relpath_empty_target_test_impl)
_relpath_empty_base_test = unittest.make(_relpath_empty_base_test_impl)
_relpath_identical_test = unittest.make(_relpath_identical_test_impl)
_relpath_absolute_test = unittest.make(_relpath_absolute_test_impl)

def cgo_godefs_test_suite(name):
    unittest.suite(
        name,
        _relpath_sibling_test,
        _relpath_nested_test,
        _relpath_same_dir_test,
        _relpath_empty_target_test,
        _relpath_empty_base_test,
        _relpath_identical_test,
        _relpath_absolute_test,
    )
