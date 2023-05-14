# E2E Tests

This folder contains tests and utilities to write and run agent end to end tests based on Pulumi.

## Development in VSCode

This is a sub-module within `datadog-agent`. VSCode will complain about the multiple `go.mod` files. While waiting for a full repo migration to go workspaces, create a go workspace file and add `test/new-e2e` to workspaces

```bash
go work init
go work use . ./test/new-e2e
```

> **Note**
> `go.work` file is currently ignored in `datadog-agent`
