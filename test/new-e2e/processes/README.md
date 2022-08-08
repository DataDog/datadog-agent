# Datadog Process Agent e2e test PoC

The following tests are based off the [ndm e2e tests](../ndm/README.md).

## Prerequisite

1. Install Pulumi
    ```bash
    brew install pulumi/tap/pulumi
    ```
2. Create a local Pulumi state manager
    ```bash
    pulumi login --local
    ```
3. Add a PULUMI_CONFIG_PASSPHRASE to your Terminal rc.
    ```bash
    export PULUMI_CONFIG_PASSPHRASE=citest
    ```
4. Install aws plugin
    ```bash
    pulumi plugin install resource aws
    ```

## Running tests

### Running all tests (docker, linux)
```bash
go test test/new-e2e/processes/*.go -v
```

### Running Docker tests
```bash
go test -run TestProcessAgentOnDocker test/new-e2e/processes/*.go -v
```

### Running Linux tests
```bash
go test -run TestProcessAgentOnLinux test/new-e2e/processes/*.go -v
```


### Manual QA

The infra that is created during the test run are torn down in the `TearDownSuite()`. The agent
and all test artifacts are torn down by `TearDownTest()`. By commenting out these functions will disable all
cleanup which will allow further inspection of the agent.

You will need to manually clean up the specific infrastructure that is not destroyed by the test.
- Linux
    ```bash
    pulumi destroy -y -s ci-agent-process-agent-linux-test
    ```
- Docker
    ```bash
    pulumi destroy -y -s ci-agent-process-agent-docker-test
    ```
