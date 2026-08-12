// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package workloadbalancing

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

const workloadBalancingMetadataHostname = "test-e2e-workload-balancing-metadata"
const workloadBalancingMetadataGroupID = "test-metadata-group01"

type workloadBalancingMetadataTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

type workloadBalancingMetadataPayload struct {
	Metadata struct {
		Enabled bool              `json:"enabled"`
		Groups  map[string]string `json:"groups"`
	} `json:"workload_balancing_metadata"`
}

// TestWorkloadBalancingMetadataSuite runs the Agent Workload Balancing Metadata e2e suite
func TestWorkloadBalancingMetadataSuite(t *testing.T) {
	// language=yaml
	agentConfig := fmt.Sprintf(`
hostname: %s
agent_workload_balancing:
    enabled: true
log_level: debug
`, workloadBalancingMetadataHostname)

	e2e.Run(t, &workloadBalancingMetadataTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)))),
	))
}

func (s *workloadBalancingMetadataTestSuite) TestWorkloadBalancingMetadata() {
	fakeClient := s.Env().FakeIntake.Client()
	payload := fmt.Sprintf(`{"group_id":%q,"active_agent":%q}`, workloadBalancingMetadataGroupID, workloadBalancingMetadataHostname)
	err := fakeClient.RCAddConfig("", state.ProductNDMAgentWorkloadBalancing, workloadBalancingMetadataGroupID, "leader", []byte(payload))
	require.NoError(s.T(), err)

	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert workload_balancing metadata")
		output := s.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "workload-balancing"}))

		var payload workloadBalancingMetadataPayload
		err := json.Unmarshal([]byte(output), &payload)
		require.NoError(c, err)

		assert.True(c, payload.Metadata.Enabled, "expected enabled to be true")
		assert.Equal(c, "active", payload.Metadata.Groups[workloadBalancingMetadataGroupID], "expected group %s to be active", workloadBalancingMetadataGroupID)
	}, 5*time.Minute, 30*time.Second)
}
