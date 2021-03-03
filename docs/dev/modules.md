# Creating a Go nested module

The Datadog Agent is not meant to be imported as a Go library, however, certains part of it can be exposed as a library by making use of [nested Go modules](https://github.com/go-modules-by-example/index/blob/master/009_submodules/README.md). This allows selectively exposing certain parts of the codebase without extracting the code to a different repository, and it avoids `replace` directive clashes.

At present the Go modules offer no public stability guarantees and are intended for internal consumption by Datadog only.

## Creating a new Go nested module

To create a new Go nested module, you will first need to determine the packages that you want to expose and their dependencies. You might need to refactor the code to have an exportable package, since the `replace` directives that we use might be incompatible with your project.
After the possible refactor and once you have listed all the packages that you want to expose and their dependencies, you will need to follow these steps for each module you want to create.

1. Create a `tools.go` file at the module root folder. You can use the one on the main folder as a starting point. This will define the tools used for CI.
1. Create a `.golangci.yml` file at the module root folder. You can use the one on the main folder as a starting point. This defines how the `golangci-lint` CI check works.
1. Create the `go.mod` and `go.sum` files at the module root folder. You can use `go mod init && go mod tidy` within the folder of the module as a starting point. Make sure the go version line matches the version on the main `go.mod` file.
1. On each module that depends on the current one, add a `require` directive with the module path with version `v0.0.0` 
1. On each module that depends on the current one, add a `replace` directive to replace the module with the repository where it lives.
1. Update the `DEFAULT_MODULES` dictionary on the `tasks/modules.py` file. You need to:
    - create a new module, specifying the path, targets, dependencies and a condition to run tests (if any) and
    - update the dependencies of other modules if they depend on this one.

## Go nested modules tooling

Go nested modules interdependencies are automatically updated when creating a release candidate or a final version, with the same tasks that update the `release.json`. For Agent version `7.X.Y` the module will have version `v0.X.Y`.

Go nested modules are tagged automatically by the `release.tag-version` invoke task, on the same commit as the main module, with a tag of the form `path/to/module/v0.X.Y`.