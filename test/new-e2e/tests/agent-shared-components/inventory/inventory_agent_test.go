// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type inventoryAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestInventoryAgentSuite(t *testing.T) {
	e2e.Run(t, &inventoryAgentSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *inventoryAgentSuite) TestInventoryDefaultConfig() {
	inventory := v.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "inventory-agent"}))
	assert.Contains(v.T(), inventory, `"feature_apm_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_logs_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_process_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_networks_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_cspm_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_cws_enabled": false`)
	assert.Contains(v.T(), inventory, `"feature_usm_enabled": false`)
}

func (v *inventoryAgentSuite) TestInventoryAllEnabled() {
	agentConfig := `logs_enabled: true
process_config:
  enabled: true
  process_collection:
    enabled: true
compliance_config:
  enabled: true`

	systemProbeConfiguration := `runtime_security_config:
  enabled: true
service_monitoring_config:
  enabled: true
network_config:
  enabled: true`

	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(string(agentConfig)),
		agentparams.WithSystemProbeConfig(string(systemProbeConfiguration)),
	}

	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentOptions...)))

	inventory := v.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "inventory-agent"}))
	assert.Contains(v.T(), inventory, `"feature_apm_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_logs_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_process_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_networks_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_cspm_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_cws_enabled": true`)
	assert.Contains(v.T(), inventory, `"feature_usm_enabled": true`)
}
