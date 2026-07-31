# Writing E2E tests

/// info
The rules a new test is reviewed against live in <<<repo("test/new-e2e/codereview_guideline.md")>>>. This page covers the framework mechanics; that file covers the expectations.
///

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
