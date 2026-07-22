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
// calls krakend's discover_config callback with the container's host and
// exposed ports, discover_config probes candidate ports in order (starting
// with krakend's default metrics port 9090) and returns an OpenMetrics check
// config for the first one that yields real krakend metrics. The fake
// container serves a non-krakend decoy on 9090 and the real krakend metrics
// on 9091, so a successful test proves discover_config actually probes and
// validates candidates rather than blindly using the first exposed port.
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
	if !assert.True(c, strings.Contains(configCheckOutput, ":9091/metrics"), "openmetrics_endpoint should point to port 9091 (the real krakend metrics endpoint, not the 9090 decoy)") {
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
	ran := false
	for name, stat := range instances {
		if len(stat.ExecutionTimes) > 0 {
			t.Logf("krakend instance %s: runs=%d", name, len(stat.ExecutionTimes))
			ran = true
			break
		}
	}
	if !assert.True(c, ran, "krakend check is configured but has not executed yet") {
		return
	}

	// Verify config.provider in inventory-checks metadata reflects that the
	// config came from the configuration-discovery path (via the Docker
	// listener), not a plain file provider.
	s.verifyKrakendCheckProvider(c)
}

// adContainerDiscoveryProvider mirrors names.ADContainerDiscovery in
// comp/core/autodiscovery/providers/names (not importable here: it lives in
// the root module, which test/new-e2e does not depend on). It is the source
// prefix that configmgr_discovery.go's rewriteSource applies to file-based
// configs resolved via configuration discovery against non-process services
// (e.g. containers, discovered via the Docker listener here).
const adContainerDiscoveryProvider = "ad-container-discovery+file"

// verifyKrakendCheckProvider checks that the krakend check has
// config.provider = adContainerDiscoveryProvider in the inventory-checks
// metadata, confirming it was resolved via configuration discovery against
// a container (as opposed to a process).
func (s *configDiscoverySuite) verifyKrakendCheckProvider(c *assert.CollectT) {
	t := s.T()

	metadataOut := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "agent", "diagnose", "show-metadata", "inventory-checks")

	var payload struct {
		CheckMetadata map[string][]map[string]interface{} `json:"check_metadata"`
	}
	if !assert.NoError(c, json.Unmarshal([]byte(metadataOut), &payload), "failed to parse inventory-checks metadata") {
		t.Logf("inventory-checks output: %s", metadataOut)
		return
	}

	instances, exists := payload.CheckMetadata["krakend"]
	if !assert.True(c, exists, "krakend should appear in inventory-checks metadata") {
		keys := make([]string, 0, len(payload.CheckMetadata))
		for k := range payload.CheckMetadata {
			keys = append(keys, k)
		}
		t.Logf("available checks in inventory metadata: %v", keys)
		return
	}
	if !assert.NotEmpty(c, instances, "krakend metadata should have at least one instance") {
		return
	}

	// Always logged (not just on failure) so the raw metadata is available for
	// manual debugging, e.g. when config.provider matches but other fields look off.
	t.Logf("krakend inventory-checks metadata: %+v", instances)

	assert.Equal(c, adContainerDiscoveryProvider, instances[0]["config.provider"],
		"krakend resolved via configuration discovery should have config.provider = %s", adContainerDiscoveryProvider)
}
