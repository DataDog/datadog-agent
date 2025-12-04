# Running E2E tests

End-to-End (E2E) tests validate complete user workflows in production-like environments with real infrastructure and external services. The Datadog Agent uses the [test-infra-definitions](https://github.com/DataDog/datadog-agent/test/e2e-framework) framework to provision and manage test environments. Tests are stored in the [test/new-e2e](../../../../test/new-e2e/) folder.

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

#### AWS Configuration

E2E tests require access to the `agent-sandbox` AWS account:

1. **Role Access**: Ensure you have the `account-admin` role on the `agent-sandbox` account
2. **AWS Keypair**: Set up an existing AWS keypair for your account
3. **Authentication**: Validate your connection with login using aws-vault:
   ```bash
   aws-vault login sso-agent-sandbox-account-admin
   ```

#### GCP Configuration (for GKE tests)

If you plan to run tests on Google Kubernetes Engine:

1. **Install GKE Plugin**: Install the GKE authentication plugin
2. **PATH Configuration**: Add the plugin to your system PATH
3. **Authentication**: Validate your connection with GCP authentication:
   ```bash
   gcloud auth application-default login
   ```

### Environment Configuration

Set up the required environment variables:

1. **Pulumi Configuration**: Set `PULUMI_CONFIG_PASSPHRASE` in your terminal rc file (`.bashrc`, `.zshrc`, etc.)
   ```bash
   export PULUMI_CONFIG_PASSPHRASE="your-random-password"
   ```
   /// tip
   Generate a secure random password using 1Password or your preferred password manager.
   You can use `security` on macOS to safely get this password.
   ///

## Setting Up the Development Environment

E2E tests should be run within a [developer environment](../../tutorials/dev/env.md) to ensure consistency and proper isolation.

### Start a Developer Environment

1. **Clone the repository** (if using local checkout):
   ```bash
   git clone https://github.com/DataDog/datadog-agent.git
   cd datadog-agent
   ```

2. **Start the environment** The following example is for an amd64 environment:
   ```bash
   dda env dev start --id devenv-amd --arch amd64
   ```

   Or use a remote clone for better isolation:
   ```bash
   dda env dev start --clone
   ```

3. **Enter the environment shell**:
   ```bash
   dda env dev shell --id devenv-amd
   ```

For detailed information about developer environments, see the [Using developer environments](../../tutorials/dev/env.md) tutorial.

## Running E2E Tests

### Basic Test Execution

E2E tests are located in the `test/new-e2e/` directory. To run a basic test use the invoke task `new-e2e-tests.run`, specifying a target folder relative to `test/new-e2e/`:

```bash
# Run a simple VM test
dda inv new-e2e-tests.run --targets=./examples --run=^TestVMSuite$
```

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
aws-vault exec sso-agent-sandbox-account-admin -- \
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
        e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake()),
    }

    e2e.Run(t, &vmSuite{}, suiteParams...)
}
```

### Available Provisioners

The framework provides several provisioners for different scenarios:

- **AWS Host**: `awshost.Provisioner*()` - Provision EC2 instances
- **Kubernetes**: `eks.Provisioner*()` - Provision EKS clusters
- **Docker**: Container-based provisioning
- **Multi-platform**: Cross-platform test scenarios

### Test Validation

E2E tests should validate complete workflows:

```go
func (v *vmSuite) TestAgentInstallation() {
    vm := v.Env().RemoteHost

    // Install agent
    _, err := vm.Execute("sudo apt-get install datadog-agent")
    v.Require().NoError(err)

    // Verify agent is running
    out, err := vm.Execute("sudo systemctl status datadog-agent")
    v.Require().NoError(err)
    v.Require().Contains(out, "active (running)")

    // Validate metric submission
    v.Eventually(func() bool {
        return v.Env().FakeIntake.GetMetricCount() > 0
    }, 30*time.Second, 1*time.Second)
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

### Test Design
- **Single Responsibility**: Each test should validate one specific workflow
- **Clear Assertions**: Use descriptive assertion messages
- **Proper Timeouts**: Set appropriate timeouts for operations
- **Resource Management**: Always clean up created resources

### Performance Considerations
- **Parallel Execution**: Design tests to run in parallel when possible
- **Resource Efficiency**: Reuse infrastructure when appropriate
- **Test Duration**: Keep individual tests under 10 minutes when possible

### Maintenance
- **Regular Updates**: Keep test environments updated with latest agent versions
- **Documentation**: Document test scenarios and expected outcomes
- **Monitoring**: Monitor test execution times and failure rates
- **Version Compatibility**: Test against supported platform versions

## See Also

- [Test Categories](../../guidelines/testing/test-categories.md) - Understanding different test types
- [Unit Testing](unit.md) - Running unit tests
- [Using Developer Environments](../../tutorials/dev/env.md) - Setting up development environments
- [test-infra-definitions](https://github.com/DataDog/datadog-agent/test/e2e-framework) - Infrastructure provisioning framework
