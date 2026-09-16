"""Bazel helpers for dd-procmgrd-lib integration tests.

Spawn-heavy integration tests are listed in BUILD.bazel and run as one
rust_test target per test filter. That gives each test its own PASS/FAIL line
in CI (including under --keep_going) and avoids cross-test pollution when
RUST_TEST_THREADS=1 still shares one process for the default monolithic target.

To isolate another test, add an entry to _DD_PROCMGRD_ISOLATED_INTEGRATION_TESTS
in BUILD.bazel: (suffix, filter) or (suffix, filter, data, extra_env).
"""

load("@rules_rust//rust:defs.bzl", "rust_test")

_LINUX_OR_WINDOWS = select({
    "@platforms//os:linux": [],
    "@platforms//os:windows": [],
    "//conditions:default": ["@platforms//:incompatible"],
})

_DD_PROCMGRD_LIB_TEST_DEPS = ["@crates//:tempfile"]

def dd_procmgrd_lib_test(name, args = [], env = {}, data = []):
    """rust_test for :dd-procmgrd-lib with shared procmgr CI settings."""
    rust_test(
        name = name,
        args = args,
        crate = ":dd-procmgrd-lib",
        data = data,
        edition = "2024",
        env = env,
        rustc_flags = ["--cfg=bazel"],
        target_compatible_with = _LINUX_OR_WINDOWS,
        deps = _DD_PROCMGRD_LIB_TEST_DEPS,
    )

def _isolated_test_entry(entry):
    """Normalize (suffix, filter[, data[, extra_env]]) to a 4-tuple."""
    suffix = entry[0]
    filter_arg = entry[1]
    data = entry[2] if len(entry) > 2 else []
    extra_env = entry[3] if len(entry) > 3 else {}
    return suffix, filter_arg, data, extra_env

def dd_procmgrd_isolated_integration_tests(name_prefix, tests, env):
    """Declare one dd_procmgrd_lib_test per isolated integration test entry.

    Args:
        name_prefix: Target name prefix, e.g. "dd-procmgrd".
        tests: List of (suffix, filter) or (suffix, filter, data, extra_env)
            tuples. filter is the Rust test function name (unique substring
            matched against module::path::name).
        env: Base environment dict merged with any per-test extra_env.

    Returns:
        List of declared target name strings (without leading colon).
    """
    declared = []
    for entry in tests:
        suffix, filter_arg, data, extra_env = _isolated_test_entry(entry)
        target = "{}_{}_test".format(name_prefix, suffix)

        # Substring filter: libtest --exact requires the full module::path::name,
        # but we only keep the function name in BUILD.bazel.
        dd_procmgrd_lib_test(
            name = target,
            args = [filter_arg],
            data = data,
            env = env | extra_env,
        )
        declared.append(target)
    return declared

def dd_procmgrd_skip_args_for_isolated(tests):
    """Build --skip args for the main target from isolated test filters.

    Args:
        tests: List of isolated test entries, same as for
            dd_procmgrd_isolated_integration_tests.

    Returns:
        List of rust_test args with one --skip per filter name (substring match).
    """
    args = []
    for entry in tests:
        _, filter_arg, _, _ = _isolated_test_entry(entry)
        args.extend(["--skip", filter_arg])
    return args
