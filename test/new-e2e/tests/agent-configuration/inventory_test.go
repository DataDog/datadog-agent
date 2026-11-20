// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

type inventoryAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestInventoryAgentSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &inventoryAgentSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *inventoryAgentSuite) TestInventoryDefaultConfig() {
	inventory := v.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "inventory-agent"}))
	assert.Contains(v.T(), inventory, `"feature_apm_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_logs_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_process_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_networks_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_traceroute_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_synthetics_collector_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_network_path_remote_config_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_network_path_connections_monitoring_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_cspm_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_cws_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_usm_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_infra_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_autoscaling_cluster_enabled": false`)

	v.assertSentFeatureFields(map[string]bool{
		"feature_infra_enabled":               true,
		"feature_autoscaling_cluster_enabled": false,
	})
}

func (v *inventoryAgentSuite) TestInventoryAllEnabled() {
	agentConfig := `logs_enabled: true
process_config:
  enabled: true
  process_collection:
    enabled: true
compliance_config:
  enabled: true
synthetics:
  collector:
    enabled: true
network_path:
  remote_config:
    enabled: true
  connections_monitoring:
    enabled: true
autoscaling:
  cluster:
    enabled: true`

	systemProbeConfiguration := `runtime_security_config:
  enabled: true
service_monitoring_config:
  enabled: true
network_config:
  enabled: true
traceroute:
  enabled: true`

	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(string(agentConfig)),
		agentparams.WithSystemProbeConfig(string(systemProbeConfiguration)),
	}

	v.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(scenec2.WithAgentOptions(agentOptions...))))

	inventory := v.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "inventory-agent"}))
	assert.Contains(v.T(), inventory, `"feature_apm_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_logs_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_process_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_networks_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_traceroute_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_synthetics_collector_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_network_path_remote_config_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_network_path_connections_monitoring_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_cspm_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_cws_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_usm_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_infra_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_autoscaling_cluster_enabled": true`)

	v.assertSentFeatureFields(map[string]bool{
		"feature_infra_enabled":               true,
		"feature_autoscaling_cluster_enabled": true,
	})
}

// assertSentFeatureFields checks the latest inventory-agent payload received by fakeintake, i.e. what
// the Agent actually sent to /api/v1/metadata. The backend derives these fields from
// full_configuration when they are absent, so a missing key must fail even when false is expected.
// The intake is not reset between tests: using the latest payload lets EventuallyWithT wait for the
// one sent after the Agent restarted with this test's configuration.
func (v *inventoryAgentSuite) assertSentFeatureFields(expected map[string]bool) {
	client := v.Env().FakeIntake.Client()
	v.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := client.GetMetadata()
		require.NoError(c, err)

		var latest *aggregator.MetadataPayload
		for _, payload := range payloads {
			if payload.Metadata == nil { // not an inventory-agent payload
				continue
			}
			if latest == nil || payload.Timestamp > latest.Timestamp {
				latest = payload
			}
		}
		require.NotNil(c, latest, "no inventory-agent payload received yet")

		for key, want := range expected {
			value, ok := latest.Metadata[key]
			require.True(c, ok, "%s missing from the latest inventory-agent payload", key)
			assert.Equal(c, want, value, key)
		}
	}, 5*time.Minute, 10*time.Second)
}
