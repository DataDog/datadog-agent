# Running E2E tests

Complete the [one-time setup](index.md#one-time-setup) before running anything on this page.

## Basic Test Execution

E2E tests are located in the `test/new-e2e/` directory. After running `dda inv e2e.setup` once, you can run them like unit tests — no `aws-vault exec` wrapping, no exported `PULUMI_CONFIG_PASSPHRASE`. The runner reads the passphrase from your local config and auto-wraps the test command with `aws-vault exec` against the configured profile.

```bash
# Run a simple VM test
dda inv new-e2e-tests.run --targets=./examples --run=^TestVMSuite$
```

If you manage AWS credentials yourself (e.g. via SAML2AWS or another tool), pass `--no-aws-vault` to skip the auto-wrap.

Replace ./examples with your subfolder.
This also supports the golang testing flag --run and --skip to target specific tests using go test syntax. See go help testflag for details.

```bash
inv new-e2e-tests.run --targets=./examples --run=TestMyLocalKindSuite/TestClusterAgentInstalled
```

You can also run it with go test, from test/new-e2e
```bash
cd test/new-e2e && go test ./examples -timeout 0 -run=^TestVMSuite$
```

While developing a test you might want to keep the remote instance alive to iterate faster. You can skip the resources deletion using dev mode with the environment variable `E2E_DEV_MODE`. You can force this in the terminal
```bash
E2E_DEV_MODE=true inv -e new-e2e-tests.run --targets ./examples --run=^TestVMSuite$
```
or for instance add it in the `go.testEnvVars` if you are using a VSCode-based IDE
```
"go.testEnvVars": {
  "E2E_DEV_MODE": "true",
}, 
```

## Test with Local Agent Packages

/// admonition | Limitations
type: warning

Local packaging is curently limited to DEB packages, only for Linux and Macos computers.
This method relies on updating an existing agent package with the local Go binaries. As a consequence, this is incompatible with tests related to the agent packaging or the python integration.
///

From a developer environment (see [Using developer environments](../../../tutorials/dev/env.md)), you can create the agent package with your local code using:
```bash
dda inv omnibus.build-repackaged-agent
```

You can then execute your E2E tests with the associated command:
```bash
# Run tests with a specific agent version
dda inv new-e2e-tests.run --targets ./examples --run TestVMSuiteEx5 --local-package $(pwd)/omnibus
```

Make sure to replace `examples` with the package you want to test and to target the test you want to run with `--run`.

## Test with Local Agent Image

/// admonition | Limitations
type: warning

This method relies on updating an existing Agent image with the local Go binaries. It only works for Docker images and must be considered as a solution for testing only.
///

Build the Agent binary and the Docker image, using this command:
```bash
dda inv [--core-opts] agent.hacky-dev-image-build [--base-image=STRING --push --signed-pull --target-image=STRING]
```

The command uses `dda inv agent.build` to generate the Go binaries. The generated image embeds this binary, a debugger and auto-completion for the agent commands.
By default, the image is names `agent` unless you override it with the `--target-image` option.

Then push the image to a registry:
```bash
# Login to ECR
aws-vault exec sso-agent-sandbox-account-admin-8h -- \
aws ecr get-login-password --region us-east-1 | \
docker login --username AWS --password-stdin 376334461865.dkr.ecr.us-east-1.amazonaws.com
# Push the image
docker push 376334461865.dkr.ecr.us-east-1.amazonaws.com/agent-e2e-tests:$USER
```

And finally, execute your E2E tests with the associated command:
```bash
# Run Ubuntu tests
inv -e new-e2e-tests.run --targets ./tests/containers \
  --run TestDockerSuite/TestDSDWithUDP \
  --agent-image 376334461865.dkr.ecr.us-east-1.amazonaws.com/agent-e2e-tests:$USER
```
