"""Runs E2E tests through `bazel run`, building only the selected ones, and running them in parallel.

`bazel run //test/new-e2e/runner:tests[/<directory>] -- <test flags>` runs the tests beneath
`test/new-e2e/tests[/<directory>]`, each one through `test2json` from its source directory, as `go test -json` would.
"""

load("@bazel_skylib//lib:paths.bzl", "paths")
load("@rules_multirun//:defs.bzl", "multirun")

_GO_TOOLCHAIN = "@rules_go//go:toolchain"

def _civisibility_impl(_settings, _attr):
    return {"@rules_go//go/toolchain:sdk_name": "go_civisibility_sdk"}

# Builds instrumented binaries without flipping the top-level configuration, so the analysis cache survives
# interleaved invocations, like `bazel run //internal/tools:gotestsum`'s.
_civisibility = transition(
    implementation = _civisibility_impl,
    inputs = [],
    outputs = ["@rules_go//go/toolchain:sdk_name"],
)

def _e2e_test_run_impl(ctx):
    test = ctx.attr.test if type(ctx.attr.test) == "Target" else ctx.attr.test[0]
    sdk = ctx.toolchains[_GO_TOOLCHAIN].sdk
    runner = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        is_executable = True,
        output = runner,
        substitutions = {
            "{{go}}": sdk.go.short_path,
            "{{goroot}}": paths.dirname(sdk.root_file.short_path),
            "{{importpath}}": "github.com/DataDog/datadog-agent/" + test.label.package,
            "{{package}}": test.label.package,
            "{{test}}": ctx.executable.test.short_path,
        },
        template = ctx.file._template,
    )
    return [DefaultInfo(
        executable = runner,
        runfiles = ctx.runfiles(
            [sdk.go],
            transitive_files = depset(transitive = [sdk.headers, sdk.libs, sdk.srcs, sdk.tools]),  # `go tool test2json`
        ).merge(test[DefaultInfo].default_runfiles),
    )]

_e2e_test_run = rule(
    implementation = _e2e_test_run_impl,
    doc = "Runs a CI Visibility-instrumented E2E test, unless `$E2E_TEST_PACKAGES` lists other packages only.",
    attrs = {
        "test": attr.label(cfg = _civisibility, doc = "The E2E `go_test`", executable = True, mandatory = True),
        "_template": attr.label(allow_single_file = True, default = ":e2e_test.sh"),
    },
    executable = True,
    toolchains = [_GO_TOOLCHAIN],
)

def e2e_tests(name, tests):
    """Declares a runner per E2E test, and a `multirun` per directory, running the tests beneath it in parallel.

    Args:
        name: Directory of the tests relative to the parent of this package, naming the `multirun` running them all,
            the other ones being named after their own directory, relative to the same parent.
        tests: Mapping of `go_test` labels to runner names, i.e. `TEST_BINARIES`.
    """
    root = native.package_name().rpartition("/")[0] + "/" + name
    directories = {}
    for test, runner in tests.items():
        _e2e_test_run(name = runner, test = test, tags = ["manual"], testonly = True)
        parts = (name + Label(test).package.removeprefix(root)).split("/")
        for i in range(1, len(parts) + 1):
            directories.setdefault("/".join(parts[:i]), []).append(runner)

    for directory, runners in directories.items():
        multirun(
            name = directory,
            commands = runners,
            jobs = 0,  # i.e. unbounded parallelism
            print_command = False,  # keep stdout parseable by gotestsum
            tags = ["manual"],
            testonly = True,
        )
