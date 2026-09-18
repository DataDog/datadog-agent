load("@rules_go//go:def.bzl", "go_test")
load(
    "//bazel/rules/go:dd_agent_go_test.bzl",
    "e2e_test_tag_set_suffix",
    "e2e_test_tag_set_tags",
    "e2e_test_tag_set_target_compatible_with",
)
load(
    "@rules_go//go/private/orchestrion:pin_files.bzl",
    "orchestrion_pin_files",
)

# Build settings contributed by the Datadog rules_go fork (Orchestrion support).
# A per-invocation `--` flag can flip them globally, but the only way to flip
# them for one target is a rule whose dependency attribute carries a
# configuration transition.
_ORCHESTRION_ENABLED = "@rules_go//go/private/orchestrion:enabled"
_ORCHESTRION_MODE = "@rules_go//go/private/orchestrion:mode"

# "test_optimization" is the fork's standard Go `testing` path: the shared
# synthetic-testmain helper bundle (built once at the stdlib level from the
# orchestrion module proxy) injects the dd-trace-go tracer, gotesting and
# integrations into every test binary. The binary remains a plain Go test
# binary, runnable outside Bazel via `go tool test2json` — which is how the
# prebuilt e2e binaries are consumed (gotest-custom on the S3 artifacts).
_ORCHESTRION_MODE_TEST_OPTIMIZATION = "test_optimization"

# Orchestrion pin files from the test/new-e2e module root, per the
# rules_test_optimization Bzlmod onboarding guide ("Manual Orchestrion Pin
# Files"). The e2e BUILD packages are nested below the module root, so the
# labels must be explicit and exported from the owning package.
_ORCHESTRION_PIN_FILES = [
    "//test/new-e2e:go.mod",
    "//test/new-e2e:go.sum",
    "//test/new-e2e:orchestrion.tool.go",
    "//test/new-e2e:orchestrion.yml",
]

def _e2e_orchestrion_transition_impl(_settings, _attr):
    return {
        _ORCHESTRION_ENABLED: True,
        _ORCHESTRION_MODE: _ORCHESTRION_MODE_TEST_OPTIMIZATION,
    }

# 1:1 transition applied to the `actual` dependency: the raw go_test (and every
# Go action it transitively compiles) is built with Orchestrion enabled, while
# everything else in the repo stays on the default configuration.
_e2e_orchestrion_transition = transition(
    implementation = _e2e_orchestrion_transition_impl,
    inputs = [],
    outputs = [
        _ORCHESTRION_ENABLED,
        _ORCHESTRION_MODE,
    ],
)

def _dd_e2e_go_test_impl(ctx):
    # The orchestrion tool repository is env-gated (enabled_by_env defaults to
    # True on orchestrion.from_source): without --config=test-optimization
    # (repo_env=DD_TEST_OPTIMIZATION_ENABLED=1) it materializes EMPTY and every
    # downstream instrumentation silently degrades to a plain, uninstrumented
    # go_test. Fail loudly instead of producing uninstrumented e2e binaries.
    if not ctx.attr._orchestrion_tool.files.to_list():
        fail("dd_e2e_go_test: the Orchestrion tool repository is empty. " +
             "Run the build with --config=test-optimization (repo_env=DD_TEST_OPTIMIZATION_ENABLED " +
             "gates the orchestrion.from_source repository in MODULE.bazel), or set " +
             "enabled_by_env = False on orchestrion.from_source to materialize it unconditionally.")

    # A transitioned attr yields a *list* of configured targets (one per output
    # configuration, even for a 1:1 transition), so unwrap it first.
    actual = ctx.attr.actual
    if type(actual) == "list":
        if len(actual) == 0:
            fail("dd_e2e_go_test: raw go_test produced no configured targets")
        actual = actual[0]
    actual_info = actual[DefaultInfo]
    exe = actual_info.files_to_run.executable
    if exe == None:
        fail("dd_e2e_go_test: target %s does not produce an executable" % ctx.attr.actual)

    # The wrapper's executable is a symlink to the raw go_test binary, built
    # under the transitioned (instrumented) configuration. `bazel build` of the
    # wrapper materializes exactly one file per test: this symlink, which
    # downstream packaging (zstd/S3) can follow.
    out = ctx.actions.declare_file(ctx.label.name)
    ctx.actions.symlink(output = out, target_file = exe)

    runfiles = ctx.runfiles()
    runfiles = runfiles.merge(actual_info.default_runfiles)
    runfiles = runfiles.merge(actual_info.data_runfiles)
    runfiles = runfiles.merge(ctx.runfiles(files = [out]))

    providers = [
        DefaultInfo(
            files = depset([out]),
            runfiles = runfiles,
            executable = out,
        ),
    ]

    # Forward the raw go_test's test environment so `bazel test` on the
    # wrapper behaves like a direct go_test run.
    if RunEnvironmentInfo in actual:
        providers.append(actual[RunEnvironmentInfo])
    return providers

_dd_e2e_go_test = rule(
    implementation = _dd_e2e_go_test_impl,
    attrs = {
        "actual": attr.label(
            mandatory = True,
            executable = True,
            cfg = _e2e_orchestrion_transition,
            doc = "The raw go_test target, built with Orchestrion instrumentation.",
        ),
        "_allowlist_function_transition": attr.label(
            default = "@bazel_tools//tools/allowlists/function_transition_allowlist",
        ),
        "_orchestrion_tool": attr.label(
            default = "@rules_go_orchestrion_tool//:orchestrion",
            doc = "Empty unless the orchestrion from_source repository materialized (env-gated).",
        ),
    },
    test = True,
    doc = """Public E2E Go test target.

    A thin wrapper around a hidden raw go_test: the transition on `actual`
    builds the raw test with Orchestrion compile-time instrumentation while the
    rest of the repository keeps the default (uninstrumented) configuration.
    The executable is a symlink to the instrumented binary, so `bazel build`
    yields the instrumented test binary directly.""",
)

def dd_e2e_go_test(
        name,
        gotags_sets = None,
        include_default = True,
        tags = None,
        target_compatible_with = None,
        **kwargs):
    """Define an E2E Go test compiled with Orchestrion instrumentation.

    Same surface as dd_agent_go_test (which stays the default for the rest of
    the repository): a default minimally-tagged test plus one variant per
    gotags set. Each variant becomes a pair of targets:

      <name>__raw        hidden raw go_test, always tagged `manual`, built
                         under the Orchestrion transition, with the module
                         pin files staged through a hidden orchestrion_pin_files
                         target in its data
      <name>             public test target wrapping the raw one

    The public wrapper keeps the `dd_agent_go_test` Bazel tag so tag-filtered
    jobs (`--test_tag_filters=dd_agent_go_test`) still select it.

    Args:
      name: Default target name and prefix for gotags-set variants.
      gotags_sets: Lists of Go build tags, such as [["zlib", "zstd"]].
      include_default: Whether to emit the minimally tagged default test.
      tags: Optional user-supplied Bazel tags (forwarded to the public target).
      target_compatible_with: Optional platform restrictions, merged with the
              gotags-set restrictions like dd_agent_go_test.
      **kwargs: Remaining attrs forwarded to each raw go_test (srcs, embed,
              deps, data, embedsrcs, …).
    """
    user_tags = tags or []
    user_tcw = [] if target_compatible_with == None else target_compatible_with

    # `visibility` applies to the public wrapper; the raw go_test stays
    # package-private. `data` is merged with the orchestrion pin target.
    kwargs = dict(kwargs)
    visibility = kwargs.pop("visibility", None)
    raw_data = list(kwargs.pop("data", None) or [])

    def _emit(target_name, gotags, variant_tags, variant_tcw):
        # Stage the module-root Orchestrion pin files as a hidden data dep of
        # the raw go_test. rules_go scans data deps for the
        # OrchestrionPinFilesInfo provider and feeds the files into the
        # synthetic-testmain, stdlib and link actions. The BUILD packages live
        # below the module root, so explicit labels are required (onboarding
        # guide: "Pass orchestrion_pin_files whenever tests live outside the
        # package that owns the pin files").
        pin_target = target_name + "__raw_orchestrion_pins"
        orchestrion_pin_files(
            name = pin_target,
            srcs = _ORCHESTRION_PIN_FILES,
            visibility = ["//visibility:private"],
        )
        go_test(
            name = target_name + "__raw",
            gotags = e2e_test_tag_set_tags(gotags),
            data = raw_data + [":" + pin_target],
            # The raw target is an implementation detail: hide it from wildcard
            # expansion (`//...`) and from tag-filtered test jobs so the public
            # wrapper is the only entry point.
            tags = user_tags + ["dd_agent_go_test", "manual"] + variant_tags,
            target_compatible_with = variant_tcw,
            visibility = ["//visibility:private"],
            **kwargs
        )
        _dd_e2e_go_test(
            name = target_name,
            actual = ":" + target_name + "__raw",
            tags = user_tags + ["dd_agent_go_test"] + variant_tags,
            target_compatible_with = variant_tcw,
            visibility = visibility,
        )

    if include_default:
        _emit(name, None, [], user_tcw)

    for gotags in gotags_sets or []:
        suffix = e2e_test_tag_set_suffix(gotags)
        _emit(
            name + "_" + suffix,
            gotags,
            ["tagset_" + suffix],
            user_tcw + e2e_test_tag_set_target_compatible_with(gotags),
        )
