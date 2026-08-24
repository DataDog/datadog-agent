# Writing E2E tests

/// info
This page covers the framework mechanics. The rules a new test is reviewed against — reliability, timing, structuring parent and child tests, keeping suites fast, CI wiring — are in <<<repo("test/new-e2e/codereview_guideline.md")>>>. Read that before writing a new suite.
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

E2E tests should validate complete workflows. With `awshost.Provisioner()` the Agent and a fakeintake are already deployed, so the test asserts on the running system rather than setting it up:

```go
func (v *vmSuite) TestAgentReportsMetrics() {
    // Verify agent is running
    out := v.Env().RemoteHost.MustExecute("sudo systemctl status datadog-agent")
    v.Require().Contains(out, "active (running)")

    // Validate metric submission. Payloads take time to arrive, so never assert
    // on them synchronously.
    v.EventuallyWithT(func(c *assert.CollectT) {
        metricNames, err := v.Env().FakeIntake.Client().GetMetricNames()
        require.NoError(c, err)
        assert.NotEmpty(c, metricNames)
    }, 2*time.Minute, 10*time.Second)
}
```

/// warning
Do not install anything from the public internet in a test body — no `apt-get install`, no `curl https://…`. It is the single largest source of E2E flakiness, and CI is losing outbound internet access. See [Test dependencies](dependencies.md).
///
