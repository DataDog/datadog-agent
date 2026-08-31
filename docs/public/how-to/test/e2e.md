# Running E2E tests

End-to-End (E2E) tests validate complete user workflows in production-like environments with real infrastructure and external services. The Datadog Agent uses the Pulumi-based <<<repo("test/e2e-framework", "E2E framework")>>> to provision and manage test environments. Tests are stored in the <<<repo("test/new-e2e", "test/new-e2e")>>> folder.

## Prerequisites

/// admonition | Datadog Employees Only
    type: warning

E2E testing requires access to Datadog's internal cloud infrastructure and is currently limited to Datadog employees. This limitation is temporary and may be expanded in the future.
///


### Software Requirements

Before running E2E tests, ensure you have the following installed:

- **Go 1.22 or later**
- **Python 3.9+**
- **dda tooling** - Install by following the [development requirements](../../setup/required.md)

### Cloud Provider Setup

You need access to the `account-admin-8h` role on the `agent-sandbox` AWS account, with the SSO profile (`sso-agent-sandbox-account-admin-8h`) already in your `~/.aws/config` and an active aws-vault session. AWS authentication is handled outside of `e2e.setup` — typically by your org's onboarding tooling, or manually with `aws-vault login`.

For Azure / GCP tests, pass `--with-azure` / `--with-gcp` when running the setup task (see below).

### One-time setup

Run the setup task once on a fresh machine. The default path is AWS-only and asks at most one question (your GitHub team, used to tag resources). It auto-creates the EC2 keypair (using your existing aws-vault session) and generates a Pulumi passphrase.

```bash
dda inv e2e.setup
```

For Azure or GCP support:

```bash
dda inv e2e.setup --with-azure --with-gcp
```

The configuration is persisted to `~/.test_infra_config.yaml` (chmod `0600`, since it contains the auto-generated Pulumi passphrase). Re-running `dda inv e2e.setup` is idempotent — it prints `✓ already configured` checks and exits.


## Running E2E Tests

### Basic Test Execution

E2E tests are located in the `test/new-e2e/` directory. After running `dda inv e2e.setup` once, you can run them like unit tests — no `aws-vault exec` wrapping, no exported `PULUMI_CONFIG_PASSPHRASE`. The runner reads the passphrase from your local config and auto-wraps the test command with `aws-vault exec` against the configured profile.

```bash
# Run a simple VM test
dda inv new-e2e-tests.run --targets=./examples --run=^TestVMSuite$
```

Replace ./examples with your subfolder.
This also supports the golang testing flag --run and --skip to target specific tests using go test syntax. See go help testflag for details.

```bash
dda inv new-e2e-tests.run --targets=./examples --run=TestMyLocalKindSuite/TestClusterAgentInstalled
```

You can also run it with go test, from test/new-e2e
```bash
cd test/new-e2e && go test ./examples -timeout 0 -run=^TestVMSuite$
```

While developing a test you might want to keep the remote instance alive to iterate faster. You can skip the resources deletion using dev mode with the environment variable `E2E_DEV_MODE`. You can force this in the terminal
```bash
E2E_DEV_MODE=true dda inv -e new-e2e-tests.run --targets ./examples --run=^TestVMSuite$
```
or for instance add it in the `go.testEnvVars` if you are using a VSCode-based IDE
```
"go.testEnvVars": {
  "E2E_DEV_MODE": "true",
}, 
```

### Test with Local Agent Packages

/// admonition | Limitations
type: warning

Local packaging is curently limited to DEB packages, only for Linux and Macos computers.
This method relies on updating an existing agent package with the local Go binaries. As a consequence, this is incompatible with tests related to the agent packaging or the python integration.
///

From a developer environment (see [Using developer environments](../../tutorials/dev/env.md)), you can create the agent package with your local code using:
```bash
dda inv omnibus.build-repackaged-agent
```

You can then execute your E2E tests with the associated command:
```bash
# Run tests with a specific agent version
dda inv new-e2e-tests.run --targets ./examples --run TestVMSuiteEx5 --local-package $(pwd)/omnibus
```

Make sure to replace `examples` with the package you want to test and to target the test you want to run with `--run`.

### Test with Local Agent Image

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
dda inv -e new-e2e-tests.run --targets ./tests/containers \
  --run TestDockerSuite/TestDSDWithUDP \
  --agent-image 376334461865.dkr.ecr.us-east-1.amazonaws.com/agent-e2e-tests:$USER
```

## Test Framework Usage

### Environment Provisioning

E2E tests use Pulumi-based provisioning to create real infrastructure:

```go
package examples

import (
    "testing"

    "github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
    "github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
    awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type vmSuite struct {
    e2e.BaseSuite[environments.Host]
}

func TestVMSuite(t *testing.T) {
    suiteParams := []e2e.SuiteOption{
        // Provisions a VM with the Agent and a fakeintake, which the test
        // below queries. A test asserting only on host state wants
        // ProvisionerNoFakeIntake(); a bare VM wants
        // ProvisionerNoAgentNoFakeIntake().
        e2e.WithProvisioner(awshost.Provisioner()),
    }

    e2e.Run(t, &vmSuite{}, suiteParams...)
}
```

### Available Provisioners

The framework provides several provisioners for different scenarios:

- **AWS Host**: `awshost.Provisioner*()` - Provision EC2 instances
- **Kubernetes**: `kindvm.Provisioner()` for a kind cluster on EC2, or `eks.Provisioner()` for EKS. Prefer kind: it provisions faster, costs less, and is more reliable
- **Docker**: `awsdocker.Provisioner()` - Agent in a container on an EC2 host
- **Multi-platform**: Azure, GCP and local provisioners under `testing/provisioners/`

### Test Validation

E2E tests should validate complete workflows:

Adding a test method to the suite above. This block needs three more imports:
`time`, `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require`.

```go
func (v *vmSuite) TestAgentRunning() {
    // The provisioner installs the agent; running a package manager inside a
    // test hits upstream mirrors and is not allowed.
    out, err := v.Env().RemoteHost.Execute("sudo systemctl status datadog-agent")
    v.Require().NoError(err)
    v.Require().Contains(out, "active (running)")

    // Payloads arrive after a real flush, so poll rather than asserting once.
    // Use require inside the callback so a failure short-circuits the iteration.
    v.EventuallyWithT(func(c *assert.CollectT) {
        names, err := v.Env().FakeIntake.Client().GetMetricNames()
        require.NoError(c, err)
        assert.NotEmpty(c, names, "no metrics received yet")
    }, 5*time.Minute, 10*time.Second)
}
```

## Test Categories and Scenarios

### Installation and Deployment Tests
- Fresh installation on clean systems
- Package manager installations (APT, YUM, MSI)
- Container deployment validation
- Kubernetes operator deployment

### Upgrade and Migration Tests
- Agent version upgrades
- Configuration migration
- Rollback scenarios
- Zero-downtime upgrades

### Platform Integration Tests
- Cloud provider integrations (AWS, Azure, GCP)
- Container runtime compatibility (Docker, containerd, CRI-O)
- Kubernetes version compatibility
- Operating system support validation

### Performance and Scale Tests
- High-throughput metric collection
- Resource consumption validation
- Memory leak detection
- Long-running stability tests

### Security and Compliance Tests
- Security configuration validation
- Compliance framework testing
- Permission and access control verification
- Secure communication validation

## Best Practices

The authoritative rules live in <<<repo("test/new-e2e/codereview_guideline.md")>>>; read it before writing a test. The highlights:

### Test Design
- **Single Responsibility**: Each test should validate one specific workflow
- **Clear Assertions**: Use descriptive assertion messages
- **Proper Timeouts**: Poll with `EventuallyWithT` instead of asserting once or sleeping
- **Resource Management**: Leave the environment as you found it, so a retry can reuse the same infrastructure

### Performance Considerations
- **Parallel Execution**: Design tests to run in parallel when possible
- **Resource Efficiency**: Reuse infrastructure when appropriate
- **Test Duration**: Keep a job under 15 minutes when it is gated on pull-request changes, and under 30-40 minutes on `main` or nightly

### Maintenance
- **Regular Updates**: Keep test environments updated with latest agent versions
- **Documentation**: Document test scenarios and expected outcomes
- **Monitoring**: Monitor test execution times and failure rates
- **Version Compatibility**: Test against supported platform versions

## See Also

- [Test Categories](../../guidelines/testing/test-categories.md) - Understanding different test types
- [Unit Testing](unit.md) - Running unit tests
- [Using Developer Environments](../../tutorials/dev/env.md) - Setting up development environments
- <<<repo("test/e2e-framework", "E2E framework")>>> - Infrastructure provisioning framework
