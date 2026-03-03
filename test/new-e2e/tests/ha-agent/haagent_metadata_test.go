// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package haagent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

type haAgentMetadataTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

type haAgentMetadataPayload struct {
	Metadata struct {
		Enabled bool   `json:"enabled"`
		State   string `json:"state"`
	} `json:"ha_agent_metadata"`
}

// TestHaAgentMetadataSuite runs the HA Agent Metadata e2e suite
func TestHaAgentMetadataSuite(t *testing.T) {
	// language=yaml
	agentConfig := `
ha_agent:
    enabled: true
config_id: ci-e2e-ha-metadata
log_level: debug
`

	e2e.Run(t, &haAgentMetadataTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)))),
	))
}

func (s *haAgentMetadataTestSuite) TestHaAgentMetadata() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert ha_agent metadata")
		output := s.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "ha-agent"}))

		var payload haAgentMetadataPayload
		err := json.Unmarshal([]byte(output), &payload)
		require.NoError(c, err)

		assert.True(c, payload.Metadata.Enabled, "expected enabled to be true")
		assert.NotEmpty(c, payload.Metadata.State, "expected state to have a value")
	}, 5*time.Minute, 30*time.Second)
}
