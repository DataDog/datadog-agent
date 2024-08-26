# Add a Go Module

The repository contains a few submodules. To add a new one and ensure it is tested, follow the following steps:


1. Create a directory for the module:
    ```
    cd ~/my_path_to/datadog-agent && mkdir mymodule
    ```


2. Initialize a new Go module:
    ```
    cd path/to/mymodule && go mod init
    ```


3.  Create a dummy root package file `doc.go`:
    ```bash
    cat >doc.go <<EOL
    // Unless explicitly stated otherwise all files in this repository are licensed
    // under the Apache License Version 2.0.
    // This product includes software developed at Datadog (https://www.datadoghq.com/).
    // Copyright 2016-present Datadog, Inc.
    package mymodule
    EOL
    ```


4.  Add `mymodule` to the `DEFAULT_MODULES` in [tasks/modules.py](https://github.com/DataDog/datadog-agent/blob/main/tasks/modules.py):
    ```python
    DEFAULT_MODULES = (
    ...,
    "path/to/mymodule": GoModule("path/to/mymodule", independent=True, should_tag=False, targets=["."]),
    )
    ```
    - `independent`: Should it be importable as an independent module?
    - `should_tag`: Should the Agent pipeline tag it?
    - `targets`: Should `go test` target specific subfolders?


5.  If you use your module in another module within `datadog-agent`, add the `require` and `replace` directives in `go.mod`.

    From the other module root, install the dependency with `go get`:
    ```
    go get github.com/DataDog/datadog-agent/path/to/mymodule
    ```
    Then add the [replace directive](https://go.dev/ref/mod#go-mod-file-replace) in the `go.mod` file:
    ```
    module github.com/DataDog/datadog-agent/myothermodule
    go 1.18
    // Replace with local version
    replace github.com/DataDog/datadog-agent/path/to/mymodule => ../path/to/mymodule
    require (
        github.com/DataDog/datadog-agent/path/to/mymodule v0.0.0-20230526143644-ed785d3a20d5
    )
    ```
    Example PR: [#17350](https://github.com/DataDog/datadog-agent/pull/17350/files)

