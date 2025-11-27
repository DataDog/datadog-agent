# Creating go modules in the Agent project

**The Datadog Agent is not meant to be imported as a Go library.**
However, certain parts of it can be exposed as a library by making use of [nested Go modules](https://github.com/go-modules-by-example/index/blob/master/009_submodules/README.md).

This allows for exposing parts of the codebase without needing to extract the code to a different repository, and helps avoid `replace` directive clashes.

/// warning
At present the Go modules offer no public stability guarantees and are intended for internal consumption by Datadog only.
///

## Creating a new module

1. Determine the packages that you want to expose and their dependencies.

    /// info
    You might need to refactor the code to have an exportable package, because the `replace` directives that we use might be incompatible with your project.
    ///

1. Create a directory for the module:

    ```bash
    cd ~/my_path_to/datadog-agent && mkdir mymodule
    ```

2. Initialize a new Go module:

    ```bash
    cd path/to/mymodule && go mod init && go mod tidy
    ```
This will create the `go.mod` and `go.sum` files in the module's root folder. **Ensure the `go version` line matches the version in the main `go.mod` file.**

1. Create a package file named `doc.go` in your new module based on this template:

    ```go
    /// doc.go

    // Unless explicitly stated otherwise all files in this repository are licensed
    // under the Apache License Version 2.0.
    // This product includes software developed at Datadog (https://www.datadoghq.com/).
    // Copyright 2025-present Datadog, Inc.
    package mymodule
    ```

2. Update the `modules.yml` file at the root of the repository, adding a section for your new module.

    See [`modules.yml`](#the-modulesyml-file) for more details. Here are a couple of example configurations:

    /// details
        type: example
        open: False

    ```yaml
    my/module:
        condition: is_linux
        used_by_otel: true
    ```

    ```yaml
    my/module:
        independent: false
        lint_targets:
        - ./pkg
        - ./cmd
        - ./comp
        targets:
        - ./pkg
        - ./cmd
        - ./comp
    ```
    ///

3. Update dependent modules. For each module depending on your new module, add:

    - A `require` directive in its `go.mod` containing the new module's path with version `v0.0.0`:

        ```
        // Other module's go.mod file
        require (
            // ...
            github.com/DataDog/datadog-agent/path/to/module v0.0.0
            // ...
        )
        ```

        /// note
        Make sure to also include any dependencies !
        ///

        /// tip
        You can do this by running `go get github.com/DataDog/datadog-agent/path/to/mymodule` from the root folder of the other module.

        This will also add `require` directives for all required dependencies and compute the `go.sum` changes.
        ///

    - A [`replace` directive](https://go.dev/ref/mod#go-mod-file-replac) in the main `go.mod` file to replace the module with the local path:

        ```
        // main go.mod file
        replace (
 	        github.com/DataDog/datadog-agent/path/to/module => ./path/to/module
        )
        ```

    /// example
    See this example PR: [#17350](https://github.com/DataDog/datadog-agent/pull/17350/files)
    ///

4. Cleanup and tidy. Run the following commands to generate the update `go.work` and `go.sum` files:

    ```bash
    dda inv modules.go-work
    go mod tidy
    ```

## Nested go module tagging and versioning

A few invoke tasks are available that help with automatically updating module versions and tags:

* `dda inv release.tag-modules`

    > Creates tags for Go nested modules for a given Datadog Agent version.

    /// info
    For Agent version `7.X.Y` the module will have version `v0.X.Y`.
    ///

* `dda inv release.update-modules`

    > Updates the internal dependencies between the different Agent nested go modules.


/// info
The `release.update-modules` task is also called automatically by the invoke tasks used as part of the release process:

* `dda inv release.create-rc`
* `dda inv release.finish`

The `release.tag-modules` task is also called by the `release.tag-version` invoke task, using the same commit as the main module, with a tag of the form `path/to/module/v0.X.Y`.
///

## The `modules.yml` file

The `modules.yml` file gathers all go module configurations.
Each module is listed even if this module has default attributes or is ignored.

For each module, you can specify:

* `default` - for modules with default attribute values
* `ignored` - for ignored modules.

To create a special configuration, the attributes of the `GoModule` class can be overriden - see the definition [here](/tasks/libs/common/gomodules.py) for the list of attributes and their details.

/// tip
This file can be linted and checked by using `dda inv modules.validate [--fix-format]`.
///

/// example
```yaml
modules:
  .:
    independent: false
    lint_targets:
    - ./pkg
    - ./cmd
    - ./comp
    test_targets:
    - ./pkg
    - ./cmd
    - ./comp
  comp/api/api/def:
    used_by_otel: true
  comp/api/authtoken: default
  test/integration/serverless/src: ignored
  tools/retry_file_dump:
    should_test_condition: never
    independent: false
    should_tag: false
```
///
