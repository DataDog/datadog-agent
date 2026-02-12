# Rust in the Datadog Agent

This document describes how Rust components are built and integrated in the Datadog Agent repository.

## Overview

The Datadog Agent uses [rules_rust](https://github.com/bazelbuild/rules_rust) for building Rust code with Bazel and [rules_rs](https://github.com/dzbarsky/rules_rs) to manage Cargo crates. This enables seamless integration with the existing Go and Python codebase, consistent toolchain management, and reproducible builds across the repository.

> **Important:** We strongly encourage using Bazel directly for all Rust operations (building, testing, dependency management) rather than Cargo. While Cargo may work for some local development tasks, Bazel is the source of truth for builds and ensures consistency with CI. All instructions in this document use Bazel commands.

## Toolchain Configuration

### Bazel Module Configuration

The Rust toolchain is configured in [MODULE.bazel](../../MODULE.bazel):

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

> **Important:** This is a global toolchain configuration that is used across the entire codebase of `datadog-agent`. The configuration in [MODULE.bazel](../../MODULE.bazel) **should not be changed** without proper testing to ensure
that all `rust` components are still working.

## Crate Management

External crates are managed via [rules_rs](https://github.com/aspect-build/rules_rs), which generates Bazel targets from `Cargo.toml` and `Cargo.lock` files.

### Central Crate Registry

All Rust component crates are registered in [deps/crates.MODULE.bazel](../../deps/crates.MODULE.bazel). Each component should have its own `crate.from_cargo` entry:

```starlark
crate = use_extension("@rules_rs//rs:extensions.bzl", "crate")

# Service discovery component
crate.from_cargo(
    name = "sdagent_crates",
    cargo_lock = "//pkg/discovery/module/rust:Cargo.lock",
    cargo_toml = "//pkg/discovery/module/rust:Cargo.toml",
    platform_triples = [
        "aarch64-unknown-linux-gnu",
        "x86_64-unknown-linux-gnu",
    ],
)
use_repo(crate, "sdagent_crates")

# Example: Another component would add its own entry
# crate.from_cargo(
#     name = "my_component_crates",
#     cargo_lock = "//pkg/my/component/rust:Cargo.lock",
#     cargo_toml = "//pkg/my/component/rust:Cargo.toml",
#     platform_triples = [
#         "aarch64-unknown-linux-gnu",
#         "x86_64-unknown-linux-gnu",
#     ],
# )
# use_repo(crate, "my_component_crates")
```

Crates are then referenced in BUILD files using the component's crate repository name: `@<repository_name>//:<crate_name>`.

> **Note:** `<repository_name>` in bazel context is part of the name on the left side from `//`. In case of `sd-agent` it is
`sdagent_crates` (value of the `name` attribute seen above in `crate.from_cargo` definition).

### Adding Dependencies to an Existing Component

1. **Add the dependency to your component's `Cargo.toml`:**
   ```toml
   [dependencies]
   serde = { version = "1.0", features = ["derive"] }
   ```
    `Cargo.toml` and `Cargo.lock` files will be synchronized automatically
    upon next `bazel build` invocation.

2. **Add the dependency to your `BUILD.bazel`:**
   ```starlark
   rust_library(
       name = "my_lib",
       # ...
       deps = [
           "@my_component_crates//:serde",
       ],
   )
   ```
3. **Invoke bazel build command for your target to trigger cargo synchronization:**
    ```bash
        # This will trigger the build of your component
        bazel build <your_component_target>
        # Additionally, you can only trigger a sync
        # if you don't want to build yet.
        bazel build @sdagent_crates//:all
    ```

3. **Don't forget to commit updated `Cargo.toml` and `Cargo.lock`**

> **Note:** we will need a mechanism to check that `Cargo.toml` and `Cargo.lock` are in sync. 
Most probably, in a form of a linting job in CI. As for now, please, do not forget to commit the lock
file.

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

### Step 2: Create Cargo.toml

There is nothing bazel specific about this.

```toml
[package]
name = "my-component"
version = "0.1.0"
edition = "2024"
license = "Apache-2.0"
rust-version = "1.91"

[lib]
name = "my_component"
crate-type = ["rlib"]  # Add "cdylib" if you need FFI

[[bin]]
name = "my-binary"
path = "src/main.rs"

[dependencies]
anyhow = "1.0"
serde = { version = "1.0", features = ["derive"] }
# ... your dependencies

[dev-dependencies]
tempfile = "3.0"
# ... test-only dependencies
```

### Step 3: Register Your Crates in the Module Configuration

Edit [deps/crates.MODULE.bazel](../../deps/crates.MODULE.bazel) to add a new `crate.from_cargo` entry for your component:

```starlark
crate = use_extension("@rules_rs//rs:extensions.bzl", "crate")

# Existing component
crate.from_cargo(
    name = "sdagent_crates",
    cargo_lock = "//pkg/discovery/module/rust:Cargo.lock",
    cargo_toml = "//pkg/discovery/module/rust:Cargo.toml",
    platform_triples = [
        "aarch64-unknown-linux-gnu",
        "x86_64-unknown-linux-gnu",
    ],
)
use_repo(crate, "sdagent_crates")

# Your new component - add this block
crate.from_cargo(
    name = "my_component_crates",
    cargo_lock = "//pkg/your/component/rust:Cargo.lock",
    cargo_toml = "//pkg/your/component/rust:Cargo.toml",
    platform_triples = [
        "aarch64-unknown-linux-gnu",
        "x86_64-unknown-linux-gnu",
    ],
)
use_repo(crate, "my_component_crates")
```

### Step 4: Generate the Lock File

```bash
    bazel build @my_components_crates//:all
```

This generates `Cargo.lock`.

### Step 5: Create BUILD.bazel

```starlark
load("@rules_rust//rust:defs.bzl", "rust_binary", "rust_library", "rust_test")

rust_library(
    name = "my_component",
    srcs = glob(["src/**/*.rs"], exclude = ["src/main.rs"]),
    crate_name = "my_component",
    edition = "2024",
    visibility = ["//visibility:public"],
    deps = [
        "@my_component_crates//:anyhow",
        "@my_component_crates//:serde",
    ],
)

rust_binary(
    name = "my-binary",
    srcs = ["src/main.rs"],
    edition = "2024",
    visibility = ["//visibility:public"],
    deps = [
        ":my_component",
        "@my_component_crates//:anyhow",
    ],
)

rust_test(
    name = "my_component_test",
    crate = ":my_component",
    edition = "2024",
    deps = [
        "@my_component_crates//:tempfile",
    ],
)
```

### Step 6: Build and Test

```bash
# Build
bazel build //pkg/your/component/rust:my_component
bazel build //pkg/your/component/rust:my-binary

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
    deps = ["@my_component_crates//:serde"],
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
    deps = ["@my_component_crates//:serde"],
)
```

### rust_binary

For executable binaries:

```starlark
rust_binary(
    name = "my-tool",
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
    deps = ["@my_component_crates//:tempfile"],  # dev-dependencies
)

# Integration tests (standalone test files)
rust_test(
    name = "integration_test",
    srcs = ["tests/integration_test.rs"],
    edition = "2024",
    data = [":my-tool"],  # Binary needed at runtime
    rustc_env = {
        "CARGO_BIN_EXE_my-tool": "$(rootpath :my-tool)",
    },
    deps = [
        "@my_component_crates//:tempfile",
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
bazel build --config=sd-agent-release //pkg/your/component/rust:my-binary
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
introduce your own `my_component.bazelrc` in [bazel/configs/](../../bazel/configs/) and
add `import %workspace%/bazel/configs/my_component.bazelrc` to [.bazelrc](../../.bazelrc) under
`Project configs` section.

## CI Integration

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
bazel query "@my_component_crates//..."
```

### Updating Dependencies

After modifying `Cargo.toml`, regenerate the lock file:

```bash
bazel build @my_component_crates//:all
```


## Existing Rust Components

| Component | Path | Crate Repository |
|-----------|------|------------------|
| sd-agent (discovery) | `pkg/discovery/module/rust/` | `@sdagent_crates` |

## Further Reading

- [rules_rust documentation](https://bazelbuild.github.io/rules_rust/) - Rust toolchain and build rules
- [rules_rs documentation](https://github.com/aspect-build/rules_rs) - Crate management
- [Bazel Rust examples](https://github.com/aspect-build/bazel-examples/tree/main/rust)
