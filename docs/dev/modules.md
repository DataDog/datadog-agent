# Creating a Go nested module

The Datadog Agent is not meant to be imported as a Go library. However, certain parts of it can be exposed as a library by making use of [nested Go modules](https://github.com/go-modules-by-example/index/blob/master/009_submodules/README.md). This exposes parts of the codebase without extracting the code to a different repository, and it avoids `replace` directive clashes.

At present the Go modules offer no public stability guarantees and are intended for internal consumption by Datadog only.

## Creating a Go nested module

Before you create a Go nested module, determine the packages that you want to expose and their dependencies. You might need to refactor the code to have an exportable package, because the `replace` directives that we use might be incompatible with your project.
After you have refactored, if needed, and listed the packages that you want to expose and their dependencies, follow these steps for each module you want to create:

1. Create `go.mod` and `go.sum` files in the module root folder. You can use `go mod init && go mod tidy` within the module folder as a starting point. Ensure the `go version` line matches the version in the main `go.mod` file.
1. On each module that depends on the current one, add a `require` directive with the module path with version `v0.0.0`.
   ```
    require (
        // ...
        github.com/DataDog/datadog-agent/path/to/module v0.0.0
        // ...
    )
    ```
1. On each module that depends on the current one, add a `replace` directive to replace the module with the repository where it lives.
    ```
    // main go.mod file
    replace (
 	    github.com/DataDog/datadog-agent/path/to/module => ./path/to/module
    )
    ```
1. Create a `module.yml` file next to `go.mod` (even if empty). See the GoModule documentation [here](tasks/libs/common/gomodule.py) for attributes that can be defined. The dependencies are computed automatically. Here are two example configurations:

    ```yaml
    condition: is_linux
    independent: true
    used_by_otel: true
    ```

    ```yaml
    lint_targets:
    - ./pkg
    - ./cmd
    - ./comp
    targets:
    - ./pkg
    - ./cmd
    - ./comp
    ```

## Go nested modules tooling

Go nested modules interdependencies are automatically updated when creating a release candidate or a final version, with the same tasks that update the `release.json`. For Agent version `7.X.Y` the module will have version `v0.X.Y`.

Go nested modules are tagged automatically by the `release.tag-version` invoke task, on the same commit as the main module, with a tag of the form `path/to/module/v0.X.Y`.
