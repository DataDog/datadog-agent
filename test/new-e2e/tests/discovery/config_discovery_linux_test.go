// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	_ "embed"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

//go:embed testdata/compose/docker-compose.fake-krakend.yaml
var fakeKrakendComposeStr string

type configDiscoverySuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestConfigDiscoverySuite(t *testing.T) {
	t.Parallel()

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(
			awsdocker.WithRunOptions(
				scendocker.WithAgentOptions(
					dockeragentparams.WithExtraComposeManifest("fake-krakend", pulumi.String(fakeKrakendComposeStr)),
				),
			),
		)),
	}
	e2e.Run(t, &configDiscoverySuite{}, options...)
}

// TestKrakendConfigDiscovery verifies that integration config discovery works
// end-to-end: the agent discovers the fake-krakend container via the Docker
// listener (matching ad_identifiers: [krakend] from the shipped auto_conf.yaml),
// calls krakend's discover_config callback with the container's host and port,
// discover_config probes :9090/metrics and returns an OpenMetrics check config,
// and the agent schedules and runs the krakend check.
func (s *configDiscoverySuite) TestKrakendConfigDiscovery() {
	t := s.T()
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		s.verifyKrakendConfigDiscovery(c)
	}, 3*time.Minute, 10*time.Second, "krakend check should be scheduled and running via config discovery")
}

func (s *configDiscoverySuite) verifyKrakendConfigDiscovery(c *assert.CollectT) {
	t := s.T()

	configCheckOutput := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "configcheck")
	if !assert.True(c, strings.Contains(configCheckOutput, "=== krakend check ==="), "krakend check should appear in configcheck") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}
	if !assert.True(c, strings.Contains(configCheckOutput, "openmetrics_endpoint"), "krakend config should have openmetrics_endpoint") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}
	if !assert.True(c, strings.Contains(configCheckOutput, ":9090/metrics"), "openmetrics_endpoint should point to port 9090") {
		t.Logf("configcheck output: %s", configCheckOutput)
		return
	}

	statusOutput := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "status", "collector", "--json")
	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	if !assert.NoError(c, err, "failed to parse collector status JSON") {
		t.Logf("status output: %s", statusOutput)
		return
	}
	instances, exists := status.RunnerStats.Checks["krakend"]
	if !assert.True(c, exists, "krakend check should be running; available: %v", getCheckNames(status.RunnerStats.Checks)) {
		return
	}
	for name, stat := range instances {
		if len(stat.ExecutionTimes) > 0 {
			t.Logf("krakend instance %s: runs=%d", name, len(stat.ExecutionTimes))
			return
		}
	}
	assert.Fail(c, "krakend check is configured but has not executed yet")
}
