# How to run unit tests

-----

The `dda inv test` command runs Go tests. It's implementation is transitioning from being a thin wrapper around [gotestsum](https://github.com/gotestyourself/gotestsum) to wrapping Bazel instead.
On Linux, `dda inv test` uses Bazel, `dda inv test-legacy` still being available for the `gotestsum` wrapper for
transitional purposes. On all other platforms, `dda inv test` uses `gotestsum`, the Bazel version being available under `dda inv test-new` instead.

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
