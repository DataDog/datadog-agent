# Add a Go Module

The repository contains a few submodules. To add a new one and ensure it is tested, follow the following steps:


1. Create a directory for the module
    ```
    cd ~/my_path_to/datadog-agent && mkdir mymodule
    ```


2. Initialise a new go module
    ```
    cd mymodule && go mod init
    ```


3.  Create a dummy root root package file `doc.go`:
    ```
    cat >doc.go <<EOL
    // Unless explicitly stated otherwise all files in this repository are licensed
    // under the Apache License Version 2.0.
    // This product includes software developed at Datadog (https://www.datadoghq.com/).
    // Copyright 2016-present Datadog, Inc.
    package mymodule
    EOL
    ```


4.  Add `mymodule` to the `DEFAULT_MODULES` in [tasks/modules.py](https://github.com/DataDog/datadog-agent/blob/main/tasks/modules.py)
    ```
    DEFAULT_MODULES = (
    ...,
    "mymodule": GoModule("mymodule", independent=True, should_tag=False, targets=["."]),
    )
    ```
    - `independent` should it be importable as an independent module?
    - `should_tag` should the agent pipeline tag it?
    - `targets` should `go test` target specific sub-folders?


5.  If you use your module in another module within `datadog-agent`, add the `require` and `replace` directives in `go.mod`.

    From the other module root, install the dependency with `go get`:
    ```
    go get github.com/DataDog/datadog-agent/mymodule
    ```
    then add the [replace directive](https://go.dev/ref/mod#go-mod-file-replace) in the `go.mod` file:
    ```
    module github.com/DataDog/datadog-agent/myothermodule
    go 1.18
    // Replace with local version
    replace github.com/DataDog/datadog-agent/mymodule => ../mymodule
    require (
        github.com/DataDog/datadog-agent/mymodule v0.0.0-20230526143644-ed785d3a20d5
    )
    ```
    Example PR: [#17350](https://github.com/DataDog/datadog-agent/pull/17350/files)

