# Test Categories

The Datadog Agent employs a comprehensive testing strategy with four distinct categories of tests, each serving a specific purpose in ensuring code quality, functionality, and reliability. This document serves as both a reference guide and decision-making framework for choosing the appropriate test type.

## Unit Tests

**Purpose**: Validate individual functions, methods, or small code units in isolation.

**When to Use Unit Tests**:

- Testing pure functions with predictable inputs/outputs.
- Validating business logic without external dependencies.
- Testing error handling and edge cases.
- Verifying data transformations and calculations.
- When you can mock all external dependencies effectively.

**Criteria**:

- **Speed**: Must execute in milliseconds (< 100ms per test).
- **Isolation**: No network calls, file system access, or external services.
- **Deterministic**: Same input always produces same output.
- **Independent**: Can run in any order without affecting other tests.

**Examples**:
```go
// Testing metric aggregation logic
func TestMetricAggregator_Sum(t *testing.T) {
    aggregator := NewMetricAggregator()
    aggregator.Add("cpu.usage", 10.0)
    aggregator.Add("cpu.usage", 15.0)
    assert.Equal(t, 25.0, aggregator.Sum("cpu.usage"))
}

// Testing configuration parsing
func TestConfig_ParseYAML(t *testing.T) {
    yamlData := `api_key: test123`
    config, err := ParseYAML([]byte(yamlData))
    assert.NoError(t, err)
    assert.Equal(t, "test123", config.APIKey)
}
```

**Common Patterns**:

- Mock external dependencies using interfaces.
- Test both happy path and error conditions.
- Use table-driven tests for multiple input scenarios.
- Focus on business logic rather than implementation details.

---

## Integration Tests

**Purpose**: Test the interaction between multiple components or validate functionality that requires external services.

**When to Use Integration Tests**:

- Testing database interactions and queries.
- Validating API client implementations.
- Testing component interactions within the same service.
- Verifying configuration loading from actual files.
- When external services are required but controllable.

**Criteria**:

- **Moderate Speed**: Should complete within seconds (< 30s per test).
- **Controlled Dependencies**: Use real services but in test environments.
- **CI Compatible**: Must work in both local and CI environments.
- **Platform Aware**: Should work on all supported platforms or skip gracefully.
- **Cleanup**: Must clean up resources after execution.

**Examples**:
```go
// Testing database integration
func TestMetricStore_SaveAndRetrieve(t *testing.T) {
    db := setupTestDB(t) // Creates isolated test database
    defer cleanupTestDB(t, db)

    store := NewMetricStore(db)
    metric := &Metric{Name: "test.metric", Value: 42.0}

    err := store.Save(metric)
    assert.NoError(t, err)

    retrieved, err := store.Get("test.metric")
    assert.NoError(t, err)
    assert.Equal(t, metric.Value, retrieved.Value)
}

// Testing HTTP client integration
func TestDatadogClient_SubmitMetrics(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "POST", r.Method)
        assert.Contains(t, r.Header.Get("Content-Type"), "application/json")
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    client := NewDatadogClient(server.URL, "test-key")
    err := client.SubmitMetrics([]Metric{{Name: "test", Value: 1.0}})
    assert.NoError(t, err)
}
```

**Requirements**:

- Use test containers or in-memory databases when possible.
- Implement proper setup/teardown procedures.
- Handle network timeouts and retries gracefully.
- Skip tests when required services are unavailable.

---

## System Tests

**Purpose**: Validate how multiple Agent components work together as a cohesive system, testing inter-component communication and workflows.

**When to Use System Tests**:

- Testing complete data pipelines (collection → processing → forwarding).
- Validating component startup and shutdown sequences.
- Testing configuration changes and reloads.
- Verifying metric collection from actual system resources.
- Testing cross-component communication (e.g., DogStatsD → Forwarder).

**Criteria**:

- **Realistic Environment**: Uses actual Agent binaries and configurations.
- **Component Integration**: Tests multiple Agent components together.
- **Moderate Isolation**: May use real system resources but in controlled manner.
- **Execution Time**: Should complete within minutes (< 5 minutes per test).
- **Environment Specific**: May require specific OS features or permissions.

**Examples**:
```go
// Testing DogStatsD metric forwarding pipeline
func TestDogStatsDMetricPipeline(t *testing.T) {
    // Start Agent with test configuration
    agent := startTestAgent(t, &Config{
        DogStatsDPort: 8125,
        APIKey: "test-key",
        FlushInterval: time.Second,
    })
    defer agent.Stop()

    // Send metric via DogStatsD
    conn, err := net.Dial("udp", "localhost:8125")
    require.NoError(t, err)
    defer conn.Close()

    _, err = conn.Write([]byte("test.metric:42|g"))
    require.NoError(t, err)

    // Verify metric was processed and forwarded
    assert.Eventually(t, func() bool {
        return agent.GetMetricCount("test.metric") > 0
    }, 5*time.Second, 100*time.Millisecond)
}

// Testing configuration reload
func TestAgentConfigReload(t *testing.T) {
    configFile := writeTestConfig(t, &Config{LogLevel: "info"})
    agent := startTestAgent(t, configFile)
    defer agent.Stop()

    // Update configuration
    updateTestConfig(t, configFile, &Config{LogLevel: "debug"})

    // Trigger reload
    err := agent.ReloadConfig()
    require.NoError(t, err)

    // Verify new configuration is active
    assert.Equal(t, "debug", agent.GetLogLevel())
}
```

**Characteristics**:

- May spawn actual Agent processes.
- Tests real configuration files and command-line arguments.
- Validates inter-process communication.
- Can test Python integration and bindings.

---

## E2E (End-to-End) Tests

**Purpose**: Validate complete user workflows in production-like environments with real infrastructure managed by
Pulumi, and external services.

**When to Use E2E Tests**:

- Testing complete deployment scenarios.
- Validating Agent behavior in different operating systems.
- Testing Kubernetes integration and autodiscovery.
- Verifying cloud provider integrations (AWS, Azure, GCP).
- Testing upgrade and rollback procedures.
- Validating security configurations and compliance.

**Criteria**:

- **Production-Like Environment**: Uses real infrastructure (VMs, containers, cloud services).
- **Complete Workflows**: Tests entire user journeys from installation to data delivery.
- **Extended Duration**: May run for hours to test long-running scenarios.
- **Infrastructure Dependencies**: Requires provisioned environments (Pulumi, Terraform).
- **Comprehensive Coverage**: Tests all supported platforms and configurations.

**Example**:

More examples can be found in the [examples](https://github.com/DataDog/datadog-agent/tree/main/test/new-e2e/examples) directory.
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

// TestVMSuite runs tests for the VM interface to ensure its implementation is correct.
func TestVMSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.ProvisionerNoAgentNoFakeIntake())}

	e2e.Run(t, &vmSuite{}, suiteParams...)
}

func (v *vmSuite) TestExecute() {
	vm := v.Env().RemoteHost

	out, err := vm.Execute("whoami")
	v.Require().NoError(err)
	v.Require().NotEmpty(out)
}
```

**Infrastructure Examples**:

- **Container Orchestration**: Testing in Kubernetes, Docker Swarm, ECS.
- **Operating Systems**: Validating on Ubuntu, CentOS, Windows Server, macOS.
- **Cloud Integrations**: Testing AWS EC2, Azure VMs, GCP Compute Engine.
- **Network Configurations**: Testing with firewalls, load balancers, service meshes.

**Test Scenarios**:

- Fresh installation and first-time setup.
- Agent updates and version migrations.
- High-availability and failover scenarios.
- Performance under load and resource constraints.
- Security configurations and compliance validation.

---

## Choosing the Right Test Type

Use this decision tree to determine the appropriate test category:

1. **Can you test it with mocked dependencies?** → **Unit Test**
1. **Does it require external services but limited scope?** → **Integration Test**
1. **Does it test multiple Agent components together?** → **System Test**
1. **Does it require production-like infrastructure?** → **E2E Test**

**Speed vs Coverage Trade-off**:

- **Unit Tests**: Maximum speed, narrow coverage.
- **Integration Tests**: Fast execution, moderate coverage.
- **System Tests**: Moderate speed, broad coverage.
- **E2E Tests**: Slower execution, maximum coverage.

**Execution Context**:

- **Unit + Integration**: Run on every commit (CI/CD pipeline).
- **System**: Run on pull requests and nightly builds.
- **E2E**: Run on releases and scheduled intervals.

Remember: A well-tested codebase uses all four categories strategically. Start with unit tests for core logic, add integration tests for external dependencies, use system tests for component interactions, and implement E2E tests for critical user workflows.
