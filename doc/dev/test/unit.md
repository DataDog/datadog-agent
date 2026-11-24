# How to run unit tests

-----

The `dda inv test` command runs Go tests via Bazel.

`dda inv test-legacy` still allows the use of the former way of running tests, which wraps [gotestsum](https://github.com/gotestyourself/gotestsum) instead of Bazel. On AIX, only `dda inv test` is available, which is equivalent to `test-legacy`, as Bazel is not supported on it.

## Test selection

The Go module to test may be selected with the `-m`/`--module` flag using a relative path, defaulting to `.`.

The `-t`/`--targets` flag is used to select the targets to test using a comma-separated list of relative paths within the given module. For example, the following command runs tests for the `pkg/collector/check` and `pkg/aggregator` root packages.

```
dda inv test --targets=pkg/collector/check,pkg/aggregator
```

/// note
If no module nor targets are set then the tests for all modules and targets are executed, which may be time-consuming.
///

## Race detection

The `-r`/`--race` flag enables Go's built-in [data race detector](https://go.dev/doc/articles/race_detector).
