# Running E2E tests

End-to-End (E2E) tests validate complete user workflows in production-like environments with real infrastructure and external services. The Datadog Agent uses the [test-infra-definitions](https://github.com/DataDog/test-infra-definitions) framework to provision and manage test environments.

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
3. **Authentication**: Login using aws-vault:
   ```bash
   aws-vault login sso-agent-sandbox-account-admin
   ```

#### GCP Configuration (for GKE tests)

If you plan to run tests on Google Kubernetes Engine:

1. **Install GKE Plugin**: Install the GKE authentication plugin
2. **PATH Configuration**: Add the plugin to your system PATH
3. **Authentication**: Authenticate with GCP:
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
   ///

## Setting Up the Development Environment

E2E tests should be run within a [developer environment](../../tutorials/dev/env.md) to ensure consistency and proper isolation.

### Start a Developer Environment

1. **Clone the repository** (if using local checkout):
   ```bash
   git clone https://github.com/DataDog/datadog-agent.git
   cd datadog-agent
   ```

2. **Start the environment**:
   ```bash
   dda env dev start
   ```

   Or use a remote clone for better isolation:
   ```bash
   dda env dev start --clone
   ```

3. **Enter the environment shell**:
   ```bash
   dda env dev shell
   ```

4. **Install development tools**:
   ```bash
   dda inv install-tools
   ```

5. **If using remote clone, fetch full Git history**:
   ```bash
   git fetch --unshallow
   ```

For detailed information about developer environments, see the [Using developer environments](../../tutorials/dev/env.md) tutorial.

## Running E2E Tests

### Basic Test Execution

E2E tests are located in the `test/new-e2e/` directory. To run a basic test:

```bash
# Run a simple VM test
go test -v ./test/new-e2e/examples/vm_test.go
```

### Test with Agent Packages

To run E2E tests with specific agent packages:

```bash
# Run tests with a specific agent version
E2E_AGENT_PACKAGE_VERSION=7.45.0 go test -v ./test/new-e2e/tests/agent/...

# Run tests with latest package
go test -v ./test/new-e2e/tests/agent/...
```

### Platform-Specific Tests

Run tests on specific platforms:

```bash
# Run Ubuntu tests
go test -v ./test/new-e2e/tests/agent/ubuntu/...

# Run Windows tests
go test -v ./test/new-e2e/tests/agent/windows/...

# Run Kubernetes tests
go test -v ./test/new-e2e/tests/kubernetes/...
```

### Container and Orchestration Tests

Test the agent in containerized environments:

```bash
# Run Docker tests
go test -v ./test/new-e2e/tests/containers/docker/...

# Run Kubernetes integration tests
go test -v ./test/new-e2e/tests/kubernetes/agent-deployment/...
```

## Test Framework Usage

### Environment Provisioning

E2E tests use Pulumi-based provisioning to create real infrastructure:

```go
package examples

import (
    "testing"

    "github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
    "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
    awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
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

## Troubleshooting

### Common Issues

**Authentication Failures**:
```bash
# Re-authenticate with AWS
aws-vault login sso-agent-sandbox-account-admin

# Verify credentials
aws sts get-caller-identity
```

**Pulumi State Issues**:
```bash
# Reset Pulumi state if needed
pulumi stack rm <stack-name>
pulumi stack init <new-stack-name>
```

**Resource Cleanup**:
E2E tests should clean up automatically, but if resources remain:
```bash
# List active stacks
pulumi stack ls

# Destroy stuck resources
pulumi destroy --stack <stack-name>
```

**Platform-Specific Issues**:
- **Linux**: Install `libnotify-bin` dependency if needed
- **macOS**: Ensure Docker Desktop is running
- **Windows**: Use WSL2 backend for containers

### Test Isolation

Each E2E test should:
- Use unique resource names to avoid conflicts
- Clean up all created resources
- Be independent of other tests
- Handle test failures gracefully

### Debugging Tips

1. **Enable verbose output**:
   ```bash
   go test -v -timeout 30m ./test/new-e2e/...
   ```

2. **Preserve infrastructure for debugging**:
   ```bash
   E2E_SKIP_CLEANUP=true go test -v ./test/new-e2e/...
   ```

3. **Run specific test cases**:
   ```bash
   go test -v -run TestSpecificCase ./test/new-e2e/...
   ```

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
- [test-infra-definitions](https://github.com/DataDog/test-infra-definitions) - Infrastructure provisioning framework
