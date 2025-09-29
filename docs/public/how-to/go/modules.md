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

1. Initialize a new Go module:
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

1. Update the `modules.yml` file at the root of the repository, adding a section for your new module.

    See the GoModule documentation [here](/tasks/libs/common/gomodules.py) for the list of possible attributes. The dependencies are computed automatically.

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

1. Update dependent modules. For each module depending on your new module, add:
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

1. Cleanup and tidy. Run the following commands to generate the update `go.work` and `go.sum` files:
    ```bash
    dda inv modules.go-work
    go mod tidy
    ```
