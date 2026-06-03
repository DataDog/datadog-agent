# pkg/util/sds

This package wraps the [Datadog Sensitive Data Scanner (SDS)](https://github.com/DataDog/dd-sensitive-data-scanner) shared library and exposes a small scanner API: create a scanner, configure it with rules and scan events (e.g. to redact sensitive data).

The package has two implementations selected at compile time:

| Build            | File               | Behaviour                                                                                                  |
| ---------------- | ------------------ | ---------------------------------------------------------------------------------------------------------- |
| default (no tag) | `scanner_nosds.go` | no-op **mock**: `Scan` returns the event unchanged, `IsReady()` is always `false`. No external dependency. |
| `-tags sds`      | `scanner.go`       | the **real** scanner, linked against the `libdd_sds` shared library.                                    |

`SDSEnabled` reflects which one is compiled in. The real scanner needs the SDS shared library at link time, which is why it is gated behind the `sds` build tag. Without the tag, the Agent compiles and runs with the mock.

## Running the tests (`dda` developer environment)

The real scanner (`scanner_test.go`, behind `//go:build sds`) needs the `libdd_sds` shared library. The recommended way is to use a `dda` developer environment (a Linux container with the full toolchain, incl. `cargo`). The `sds.build-library` task builds the shared library and installs it into `dev/lib`, which is exactly where the rtloader/cgo link path (`-L.../dev/lib`) points — so no extra linker flags are needed.

> Important: build the library **inside the same environment** where you run the tests. The dev environment is a Linux container, so it produces `libdd_sds.so`; a library built on a macOS host (`.dylib`) will not satisfy the Linux linker (you'll see `ld: cannot find -ldd_sds_go`).

1. Start and enter the developer environment (replace the id with your own):

```bash
dda env dev start --id data-security-default
dda env dev shell --id data-security-default
```

2. Inside the environment (working dir is the repo root, e.g. `/repos/datadog-agent`), build the shared library and run this package's tests with the `sds` tag:

```bash
# build & install libdd_sds.so into dev/lib (clones + cargo build, ~1 min)
dda inv -- sds.build-library

# run only this package's tests, with the sds tag enabled
dda inv -- test --targets ./pkg/util/sds --include-sds
```

Expected result:

```
✓  pkg/util/sds
DONE 5 tests in 0.014s
All tests passed
```

- `--targets ./pkg/util/sds` restricts the run to this package.
- `--include-sds` appends the `sds` build tag (see `compute_build_tags_for_flavor` in `tasks/build_tags.py`), which compiles `scanner.go` / `scanner_test.go` against the real library instead of the mock.

To build the whole Agent against the real library, use `dda inv -- agent.build --include-sds`. On Windows the library is not built and the mock is used.
