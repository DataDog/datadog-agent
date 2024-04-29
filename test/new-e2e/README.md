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

## Running new-e2e tests locally

/!\ When running your tests locally it will run tests with the latest stable version of the agent, not with the changes you have locally. Because running the test requires a complete build of the agent.
As a workaround, you can run tests against the version of the agent built in an already run pipeline, to do it simply add the environment variable `E2E_PIPELINE=<pipeline-id>` to the command you use to run tests

### Installer tests

Installer tests are tests owned by Agent Build And Releases. They mostly test the installation of the agent on different platforms with different methods. These tests are defined in `./tests/agent-platform`.
The following are considered as installer tests:
- `./tests/agent-platform/install-script`: Test the installation of the agent using the install script
- `./tests/agent-platform/step-by-step`: Test the installation of the agent without the install script, following the manual installation described on the official documentation ([for example](https://app.datadoghq.com/account/settings/agent/latest?platform=debian))
- `./tests/agent-platform/upgrade`: Test the upgrade of the agent using the install script

All these tests can be executed locally with the following command:
`aws-vault exec sso-agent-sandbox-account-admin -- inv new-e2e-tests.run --targets <test folder path> --osversion '<comma separated os version list>' -platform '<debian/centos/suse/ubuntu>' --arch <x86_64/arm64>`

The available os versions can be found in the file `./tests/agent-platform/platforms/platforms.json`
