# DotSlash tools in Bazel

The adapter separates manifest declarations from runtime selection through a toolchain. The current runtime uses the host DotSlash executable and downloaded tool payloads outside Bazel's declared inputs, so consuming actions run locally without sandboxing or remote caching. Bazel's normal local incremental action cache still applies.

`dotslash_tool` exposes a manifest through `DotSlashToolInfo`. It performs no parsing, downloading, or runtime selection and requires no Windows shim. Add a declaration only when a Bazel action consumes it; the formatting fixture currently uses Buildifier. Targets in `tools/bin` have a `_tool` suffix so their names do not collide with the source manifests. They can be queried or built without a working DotSlash executable.

```starlark
load("//bazel/dotslash:defs.bzl", "dotslash_tool")

dotslash_tool(
    name = "example_tool",
    manifest = "example",
    visibility = ["//visibility:public"],
)
```

Run tools directly from the activated shell. Build actions consume their manifest declarations through the adapter:

```sh
buildifier -version
vault version
bazel build //tools/bin:buildifier_tool
bazel build //bazel/dotslash/tests:formatted
```

Full mise activation selects the DotSlash executable locally; use `mise exec -- bazel ...` in an unactivated developer shell. Other environments must provide the pinned DotSlash executable on `PATH`. The host discovery repository reads the pin from `mise.toml`, tracks the DotSlash executable path, checks its version, and registers `@dotslash_host//:toolchain`. Reading that configuration does not require mise. Invalid DotSlash configuration and missing or mismatched executables fail when a consumer analyzes that runtime, without preventing manifest declarations, queries, or unrelated builds. The normal DotSlash cache is used; an isolated cache can be selected with `--repo_env=DOTSLASH_CACHE=/absolute/path`.

For build rules, load `DOTSLASH_EXEC_GROUP`, `DotSlashToolInfo`, and `dotslash_run` from `//bazel/dotslash:defs.bzl`. Declare `exec_groups = {"dotslash": DOTSLASH_EXEC_GROUP}` on the rule and use `providers = [DotSlashToolInfo]` on its tool attribute. The manifest is platform-independent and needs no execution transition. Executable dependencies such as a wrapper must use `cfg = config.exec("dotslash")` to match the runtime. Invoke the tool through `dotslash_run(ctx, tool, outputs, arguments, inputs)` with a depset of inputs and ordinary strings or Args objects for arguments. The [formatting fixture](tests/fixtures.bzl) provides a complete consumer example.

The execution group resolves `//bazel/dotslash:toolchain_type`. A runtime supplies `executable`, declared `inputs` and `tools`, `env`, `execution_requirements`, and `use_default_shell_env`. The host implementation supplies the DotSlash executable path and declares discovery metadata containing its validated version, path, and environment as its action input. Discovery watches `mise.toml`, but the configuration file itself is not an action input: unrelated edits preserve action reuse, while changes to the effective runtime configuration invalidate actions. Its toolchain registration restricts selection to the host OS and CPU.

The helper declares the manifest and runtime inputs, passes arguments directly, and applies the selected runtime's environment and execution requirements to the consuming action. The host runtime requires `execution_requirements = {"local": "1"}`. Do not substitute a plain `ctx.actions.run` or assume an executable dependency propagates these requirements. An optional wrapper receives the complete DotSlash command as argv; the formatting fixture uses a Bazel-managed Python wrapper to connect declared files to Buildifier's standard streams.

Run `python bazel/dotslash/tests/integration_tests.py` with Bazelisk and the pinned DotSlash executable on `PATH`. The suite checks action execution, caching, runtime selection, and direct Buildozer edits. CI runs it on all supported native platforms.
