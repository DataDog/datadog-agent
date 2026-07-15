"""Tests for release_json repository helpers."""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")
load(":release_json.bzl", "read_effective_release_json")

def _fake_path(label):
    return label

def _make_fake_read(contents):
    def _fake_read(path):
        if path != "fake/release.json":
            fail("unexpected path: %s" % path)
        return contents

    return _fake_read

def _make_fake_getenv(env):
    def _fake_getenv(key, default = None):
        return env.get(key, default)

    return _fake_getenv

def _fake_rctx(env, release_json_contents):
    return struct(
        path = _fake_path,
        read = _make_fake_read(release_json_contents),
        getenv = _make_fake_getenv(env),
    )

def _read_effective_release_json_applies_dependency_overrides_impl(ctx):
    env = unittest.begin(ctx)
    release_json_contents = json.encode({
        "base_branch": "main",
        "current_milestone": "7.81.0",
        "dependencies": {
            "OVERRIDDEN_DEPENDENCY": "to-override",
            "UNCHANGED_DEPENDENCY": "stable",
            "EMPTY_OVERRIDE": "not-overridden",
        },
    })

    release_json = read_effective_release_json(
        _fake_rctx({"OVERRIDDEN_DEPENDENCY": "from-env", "EMPTY_OVERRIDE": ""}, release_json_contents),
        "fake/release.json",
    )

    asserts.equals(
        env,
        "from-env",
        release_json["dependencies"]["OVERRIDDEN_DEPENDENCY"],
    )
    asserts.equals(
        env,
        "stable",
        release_json["dependencies"]["UNCHANGED_DEPENDENCY"],
    )
    asserts.equals(
        env,
        "not-overridden",
        release_json["dependencies"]["EMPTY_OVERRIDE"],
    )
    asserts.equals(env, "main", release_json["base_branch"])

    return unittest.end(env)

_read_effective_release_json_applies_dependency_overrides_test = unittest.make(
    _read_effective_release_json_applies_dependency_overrides_impl,
)

def release_json_test_suite(name):
    unittest.suite(
        name,
        _read_effective_release_json_applies_dependency_overrides_test,
    )
