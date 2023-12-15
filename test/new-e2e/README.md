# E2E Tests

This folder contains tests and utilities to write and run agent end to end tests based on Pulumi.

## Documentation

See https://pkg.go.dev/github.com/DataDog/datadog-agent/test/new-e2e@main/pkg/utils/e2e.

## Development in VSCode

This is a sub-module within `datadog-agent`. VSCode will complain about the multiple `go.mod` files. While waiting for a full repo migration to go workspaces, create a go workspace file and add `test/new-e2e` to workspaces

```bash
go work init
go work use . ./test/new-e2e
```

## Use VsCode tasks to wrap aws-vault

The `agent-sandbox: test current file` can be used to launch test on a file withtout having to launch the whole VsCode wrapped by aws-vault exec. To use it copy the `.template` files in `.vscode` and remove the `.template` extension. 
You need to open the `new-e2e` folder

> **Note** > `go.work` file is currently ignored in `datadog-agent`
