// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package haagent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type haAgentMetadataTestSuite struct {
	e2e.BaseSuite[environments.Host]
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
		awshost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
	))
}

func (s *haAgentMetadataTestSuite) TestHaAgentMetadata() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert ha_agent metadata")
		output := s.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "ha-agent"}))

		assert.Contains(c, output, `"enabled": true`)
		assert.Contains(c, output, `"state": "active"`)
	}, 5*time.Minute, 30*time.Second)
}
