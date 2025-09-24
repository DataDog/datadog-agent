# multitool

An ergonomic approach to defining a single tool target that resolves to a matching os and CPU architecture implementation of the tool.

This implementation borrows heavily from Mark Elliot's rules_multitool.

## Usage

Define a [lockfile](lockfile.schema.json) that references the tools to load:

```json
{
  "$schema": "https://raw.githubusercontent.com/theoremlp/rules_multitool/main/lockfile.schema.json",
  "tool-name": {
    "binaries": [
      {
        "kind": "file",
        "url": "https://...",
        "sha256": "sha256 of the file",
        "os": "linux|macos|windows",
        "cpu": "x86_64|arm64"
      }
    ]
  }
}
```

The lockfile supports the following binary kinds:

- **file**: the URL refers to a file to download

  - `sha256`: the sha256 of the downloaded file
  - `headers`: (optional) a string dictionary of headers to pass to the downloader

- **archive**: the URL referes to an archive to download, specify additional options:

  - `file`: executable file within the archive
  - `sha256`: the sha256 of the downloaded archive
  - `type`: (optional) the kind of archive, as in https://bazel.build/rules/lib/repo/http#http_archive-type
  - `headers`: (optional) a string dictionary of headers to pass to the downloader

- **pkg**: the URL refers to a MacOS pkg archive to download, specify additional options:

  - `file`: executable file within the archive
  - `sha256`: the sha256 of the downloaded pkg archive
  - `headers`: (optional) a string dictionary of headers to pass to the downloader

### Bazel Module Usage

Once your lockfile is defined, load the extension in your MODULE.bazel and create a hub that refers to your lockfiles:

```python
# Set up prebuilt binaries
multitool = use_extension("//bazel/multitool:extension.bzl", "multitool")
multitool.hub(lockfile = "//bazel:prebuilt_buildtools.json")
multitool.hub(lockfile = "//bazel:prebuilt_jq.json")
use_repo(multitool, "multitool")
register_toolchains("@multitool//toolchains:all")
```

Tools may then be accessed using `@multitool//tools/tool-name`.

It's safe to call `multitool.hub(...)` multiple times, with multiple lockfiles. All lockfiles will be combined with a last-write-wins strategy.

Lockfiles defined across modules and applying to the same hub (including implicitly to the default "multitool" hub) will be combined such that the priority follows a breadth-first search originating from the root module.

It's possible to define multiple multitool hubs to group related tools together. To define an alternate hub:

```python
multitool.hub(hub_name = "alt_hub", lockfile = "//:other_tools.lock.json")
use_repo(multitool, "alt_hub")

# register the tools from this hub
register_toolchains("@alt_hub//toolchains:all")
```

These alternate hubs also combine lockfiles according to the hub_name and follow the same merging rules as the default hub.

## Disabled features

These features are available to be turned on, but they add complexity it is not clear we need.


### Running tools in the current working directory

When running `@multitool//tools/tool-name`, Bazel will execute the tool at the root of the runfiles tree due to https://github.com/bazelbuild/bazel/issues/3325.

It's possible to workaround this:
- `bazel run --run_under="cd $PWD &&" @multitool//tools/[tool-name] -- $PWD_relative_paths`
- `bazel run @multitool//tools/[tool-name] -- $PWD/file`
- To run a tool in the current working directory, use the convenience target `@multitool//tools/[tool-name]:cwd`.

And similarly for workspace_root.

- To run a tool in the Bazel module or workspace root, use the convenience target `@multitool//tools/[tool-name]:workspace_root`.

Alternatively, consider using https://registry.build/github/buildbuddy-io/bazel_env.bzl to put tools on the `PATH`.
