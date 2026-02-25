# Rust in the Datadog Agent

This document describes how Rust components are built and integrated in the Datadog Agent repository.

## Overview

The Datadog Agent uses [rules_rust](https://github.com/bazelbuild/rules_rust) for building Rust code with Bazel and [rules_rs](https://github.com/dzbarsky/rules_rs) to manage Cargo crates. This enables seamless integration with the existing Go and Python codebase, consistent toolchain management, and reproducible builds across the repository.

> **Important:** We strongly encourage using Bazel directly for all Rust operations (building, testing, except Cargo.toml management) rather than Cargo. While Cargo may work for some local development tasks, Bazel is the source of truth for builds and ensures consistency with CI. All instructions in this document use Bazel commands.

## Toolchain Configuration

### Bazel Module Configuration

The Rust toolchain is configured in [MODULE.bazel](/MODULE.bazel):

```starlark
bazel_dep(name = "rules_rust", version = "0.68.1")

rust = use_extension("@rules_rust//rust:extensions.bzl", "rust")
rust.toolchain(
    edition = "2024",
    versions = ["1.92.0"],
)
use_repo(rust, "rust_toolchains")

register_toolchains("@rust_toolchains//:all")
```

This configuration:
- Uses **Rust 2024 edition** as the default
- Pins to **Rust 1.92.0** for reproducible builds
- Registers toolchains for all supported platforms

> **Important:** This is a global toolchain configuration that is used across the entire codebase of `datadog-agent`. The configuration in [MODULE.bazel](/MODULE.bazel) **should not be changed** without proper testing to ensure
that all `rust` components are still working.

## Crate Management

All external Rust crates are managed **centrally** through a single [Cargo workspace](https://doc.rust-lang.org/cargo/reference/workspaces.html) defined in the root [Cargo.toml](/Cargo.toml). Individual components **must not** declare their own dependency versions — all versions live in the root `[workspace.dependencies]` section, and component `Cargo.toml` files reference them with `.workspace = true`.

> **Important:** Do not add crate versions directly in a component's `Cargo.toml`. Every external dependency must be declared in the root [Cargo.toml](/Cargo.toml) under `[workspace.dependencies]`. This ensures consistent versions across all Rust components, a single `Cargo.lock`, and a single source of truth for Bazel crate resolution.

### How It Works

The root [Cargo.toml](/Cargo.toml) defines three things:

1. **`[workspace]`** — lists all Rust component directories as `members`
2. **`[workspace.dependencies]`** — the single place where all external crate versions are pinned
3. **`[workspace.package]`** — shared metadata (`edition`, `license`, `rust-version`) inherited by all members

A component's `Cargo.toml` then references workspace dependencies rather than specifying versions:

```toml
# Component Cargo.toml — NO version numbers here
[package]
name = "my_component"
version = "0.1.0"
edition.workspace = true
license.workspace = true
rust-version.workspace = true

[dependencies]
anyhow.workspace = true
serde.workspace = true
# When a component needs specific features, add them on top of the workspace version:
tokio = { workspace = true, features = ["macros", "rt-multi-thread", "signal"] }
```

This produces a single `Cargo.lock` at the repository root — all components share the same resolved dependency graph.

### Bazel Integration

The workspace is registered once in [deps/crates.MODULE.bazel](/deps/crates.MODULE.bazel), pointing to the root `Cargo.toml` and `Cargo.lock`:

```starlark
crate = use_extension("@rules_rs//rs:extensions.bzl", "crate")
crate.from_cargo(
    name = "crates",
    cargo_lock = "//:Cargo.lock",
    cargo_toml = "//:Cargo.toml",
    platform_triples = [
        "aarch64-unknown-linux-gnu",
        "x86_64-unknown-linux-gnu",
    ],
    validate_lockfile = True,
)
use_repo(crate, "crates")
```

All components reference crates from this single repository: `@crates//:<crate_name>`. There is intentionally only one `crate.from_cargo` entry — do not add per-component entries.

### Adding Dependencies to an Existing Component

1. **Add the dependency version to the root [Cargo.toml](/Cargo.toml)** under `[workspace.dependencies]` (skip if the crate is already listed):
   ```toml
   [workspace.dependencies]
   serde = { version = "1.0", features = ["derive"] }
   ```

2. **Reference it in your component's `Cargo.toml`** using `.workspace = true` (never a version number):
   ```toml
   [dependencies]
   serde.workspace = true
   ```

3. **Add the dependency to your `BUILD.bazel`:**
   ```starlark
   rust_library(
       name = "my_lib",
       # ...
       deps = [
           "@crates//:serde",
       ],
   )
   ```

4. **Regenerate `Cargo.lock`:**
    ```bash
    cargo generate-lockfile
    ```

5. **Commit both the root `Cargo.toml` and `Cargo.lock`**

## Adding a New Rust Component

Follow these steps to add a new Rust component to the repository.

### Step 1: Create the Directory Structure

```
<path_to_your_component>
├── BUILD.bazel
├── Cargo.toml
├── src/
│   ├── lib.rs
│   └── main.rs  # if building a binary
└── tests/       # optional integration tests
```

> **Note:** The component directory does **not** contain a `Cargo.lock` — the single lock file lives at the repository root.

### Step 2: Add Your Component to the Cargo Workspace

Edit the root [Cargo.toml](/Cargo.toml):

1. **Register your component as a workspace member:**
   ```toml
   [workspace]
   members = [
       "pkg/discovery/module/rust",
       "pkg/procmgr/rust",
       "pkg/your/component/rust",  # Add your component here
   ]
   ```

2. **Add any new crate versions** to `[workspace.dependencies]` (all external crate versions must be declared here):
   ```toml
   [workspace.dependencies]
   # ... existing deps ...
   my_new_dep = "1.0"
   ```

### Step 3: Create Your Component's `Cargo.toml`

The component `Cargo.toml` must **not** contain any dependency version numbers. Use `.workspace = true` to inherit versions from the root:

```toml
[package]
name = "my_component"
version = "0.1.0"
edition.workspace = true
license.workspace = true
rust-version.workspace = true

[lib]
name = "my_component"
crate-type = ["rlib"]  # Add "cdylib" if you need FFI

[[bin]]
name = "my_binary"
path = "src/main.rs"

[dependencies]
anyhow.workspace = true
serde.workspace = true
# When you need specific features on top of the workspace-declared version:
tokio = { workspace = true, features = ["macros", "rt-multi-thread"] }

[dev-dependencies]
tempfile.workspace = true
```

> **Do not** add `version = "..."` to dependencies in component `Cargo.toml` files. If the crate you need is not yet in the root `[workspace.dependencies]`, add it there first.

### Step 4: Regenerate the Lock File

```bash
cargo generate-lockfile
```

> **Note:** You must run `cargo generate-lockfile` (or `cargo build`) whenever you change any `Cargo.toml`. If `Cargo.lock` is out of sync, Bazel will report an error:
> ```
> ERROR: Cargo.lock out of sync: sd-agent requires clap ^4.5.58 but Cargo.lock has 4.5.51.
> ```

### Step 5: Create BUILD.bazel

All components share the single `@crates` repository for external dependencies:

```starlark
load("@rules_rust//rust:defs.bzl", "rust_binary", "rust_library", "rust_test")

rust_library(
    name = "my_component",
    srcs = glob(["src/**/*.rs"], exclude = ["src/main.rs"]),
    crate_name = "my_component",
    edition = "2024",
    visibility = ["//visibility:public"],
    deps = [
        "@crates//:anyhow",
        "@crates//:serde",
    ],
)

rust_binary(
    name = "my_binary",
    srcs = ["src/main.rs"],
    edition = "2024",
    visibility = ["//visibility:public"],
    deps = [
        ":my_component",
        "@crates//:anyhow",
    ],
)

rust_test(
    name = "my_component_test",
    crate = ":my_component",
    edition = "2024",
    deps = [
        "@crates//:tempfile",
    ],
)
```

### Step 6: Build and Test

```bash
# Build
bazel build //pkg/your/component/rust:my_component
bazel build //pkg/your/component/rust:my_binary

# Test
bazel test //pkg/your/component/rust:my_component_test
```

## Build Target Types

### rust_library

For Rust libraries (produces `.rlib`):

```starlark
rust_library(
    name = "my_lib",
    srcs = glob(["src/**/*.rs"]),
    crate_name = "my_lib",
    edition = "2024",
    deps = ["@crates//:serde"],
)
```

### rust_shared_library

For C-compatible shared libraries (produces `.so`/`.dylib`), useful for FFI with Go via cgo:

```starlark
rust_shared_library(
    name = "libmy_lib",
    srcs = glob(["src/**/*.rs"]),
    crate_name = "my_lib",
    crate_root = "src/lib.rs",
    edition = "2024",
    deps = ["@crates//:serde"],
)
```

### rust_binary

For executable binaries:

```starlark
rust_binary(
    name = "my_tool",
    srcs = ["src/main.rs"],
    edition = "2024",
    deps = [":my_lib"],
)
```

### rust_test

For unit and integration tests:

```starlark
# Unit tests (embedded in library)
rust_test(
    name = "my_lib_test",
    crate = ":my_lib",
    edition = "2024",
    deps = ["@crates//:tempfile"],  # dev-dependencies
)

# Integration tests (standalone test files)
rust_test(
    name = "integration_test",
    srcs = ["tests/integration_test.rs"],
    edition = "2024",
    data = [":my_tool"],  # Binary needed at runtime
    rustc_env = {
        "CARGO_BIN_EXE_my_tool": "$(rootpath :my_tool)",
    },
    deps = [
        "@crates//:tempfile",
    ],
)
```

## Platform Restrictions

To restrict targets to specific platforms, use `target_compatible_with`:

```starlark
rust_library(
    name = "linux_only_lib",
    # ...
    target_compatible_with = [
        "@platforms//os:linux",
    ],
)
```

## Release Builds

For optimized release builds with size optimization, use the `sd-agent-release` config:

```bash
bazel build --config=sd-agent-release //pkg/your/component/rust:my_binary
```

This enables:
- Fat LTO (Link-Time Optimization)
- Size optimization (`opt-level=z`)
- Symbol stripping
- Panic abort (no stack unwinding)

For custom release profiles, add to `bazel/configs/` and import in `.bazelrc`.

>**Note:** right now we only have a release configuration that is sd-agent specific.
However, if we identify that future components want to utilize the same configuration
it can be promoted to the global `datadog-agent-release` configuration. For now, please,
introduce your own `my_component.bazelrc` in [bazel/configs/](/bazel/configs/) and
add `import %workspace%/bazel/configs/my_component.bazelrc` to [.bazelrc](/.bazelrc) under
`Project configs` section.

## CI Integration
> **TODO:** Describe how to add rust build to CI.

In CI (with `--config=ci`), Rust builds automatically run:

- **Clippy** checks via `rust_clippy_aspect`
- **Rustfmt** checks via `rustfmt_aspect`

Configuration from `.bazelrc`:
```
build:ci --aspects=@rules_rust//rust:defs.bzl%rust_clippy_aspect
build:ci --output_groups=+clippy_checks
build:ci --aspects=@rules_rust//rust:defs.bzl%rustfmt_aspect
build:ci --output_groups=+rustfmt_checks
```

>**Note:** you can also enable these checks for your local development (for instance, to format code automatically). To do so create `user.bazelrc` file in the root of the project and just add the same flags without configuration name. This enforces unconditional usage of the flags for arbitrary `build` invocation:
```starlark
build --aspects=@rules_rust//rust:defs.bzl%rust_clippy_aspect
build --output_groups=+clippy_checks
build --aspects=@rules_rust//rust:defs.bzl%rustfmt_aspect
build --output_groups=+rustfmt_checks
```

## Local Development Tips

### Common Bazel Commands

```bash
# Build a target
bazel build //pkg/your/component/rust:my_component

# Run tests
bazel test //pkg/your/component/rust:my_component_test

# Build with verbose output
bazel build --verbose_failures //pkg/your/component/rust:...

# Query dependencies
bazel query "deps(//pkg/your/component/rust:my_lib)"

# Check crate resolution
bazel query "@crates//..."
```

### Updating Dependencies

After modifying any `Cargo.toml`, regenerate the lock file from the repository root:

```bash
# Alternatively, you can use cargo build command to do the same
cargo generate-lockfile
```

Bazel will fail if `Cargo.toml` and `Cargo.lock` are out of sync:
```
ERROR: Cargo.lock out of sync: sd-agent requires clap ^4.5.58 but Cargo.lock has 4.5.51.
```

## Further Reading

- [rules_rust documentation](https://bazelbuild.github.io/rules_rust/) - Rust toolchain and build rules
- [rules_rs documentation](https://github.com/dzbarsky/rules_rs) - Crate management
