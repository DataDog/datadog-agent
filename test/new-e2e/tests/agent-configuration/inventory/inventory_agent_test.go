// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventory

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type inventoryAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestInventoryAgentSuite(t *testing.T) {
	e2e.Run(t, &inventoryAgentSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}

// InventoryAgentPayload represents the structure of the inventory-agent metadata payload
type InventoryAgentPayload struct {
	Hostname  string                 `json:"hostname"`
	Timestamp int64                  `json:"timestamp"`
	Metadata  map[string]interface{} `json:"agent_metadata"`
	UUID      string                 `json:"uuid"`
}

// DiagnosisPayload represents the structure of connectivity checker diagnostics
type DiagnosisPayload struct {
	Status      string            `json:"status"`
	Description string            `json:"description"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// DiagnosticsSection represents the diagnostics section in inventory agent metadata
type DiagnosticsSection struct {
	Connectivity []DiagnosisPayload `json:"connectivity"`
}

func getInventoryAgentOutput(v *inventoryAgentSuite) string {
	// Use the diagnose show-metadata inventory-agent command
	inventory := v.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "inventory-agent"}))
	v.T().Logf("Inventory agent command output: %s", inventory)
	return inventory
}

func unmarshalInventoryAgent(s string) *InventoryAgentPayload {
	var payload InventoryAgentPayload
	err := json.Unmarshal([]byte(s), &payload)
	if err != nil {
		return nil
	}
	return &payload
}

func getDiagnosticsFromPayload(payload *InventoryAgentPayload) *DiagnosticsSection {
	if payload == nil || payload.Metadata == nil {
		return nil
	}

	diagnosticsRaw, exists := payload.Metadata["diagnostics"]
	if !exists {
		return nil
	}

	diagnosticsBytes, err := json.Marshal(diagnosticsRaw)
	if err != nil {
		return nil
	}

	var diagnostics DiagnosticsSection
	err = json.Unmarshal(diagnosticsBytes, &diagnostics)
	if err != nil {
		return nil
	}

	return &diagnostics
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

// Connectivity checker tests
func (v *inventoryAgentSuite) TestConnectivityDefaultSite() {
	params := agentparams.WithAgentConfig("inventories_diagnostics_enabled: true")
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))
	inventory := getInventoryAgentOutput(v)

	inventoryPayload := unmarshalInventoryAgent(inventory)
	require.NotNil(v.T(), inventoryPayload)

	// Check that the payload contains expected metadata fields
	assert.NotEmpty(v.T(), inventoryPayload.Hostname)
	assert.NotEmpty(v.T(), inventoryPayload.UUID)
	assert.NotNil(v.T(), inventoryPayload.Metadata)

	// Check that diagnostics section exists and contains connectivity checks
	diagnostics := getDiagnosticsFromPayload(inventoryPayload)
	require.NotNil(v.T(), diagnostics, "Diagnostics section should be present")
	require.NotEmpty(v.T(), diagnostics.Connectivity, "Connectivity diagnostics should be present")

	// Check that connectivity diagnostics contain expected information
	for _, diagnosis := range diagnostics.Connectivity {
		assert.NotEmpty(v.T(), diagnosis.Status)
		assert.NotEmpty(v.T(), diagnosis.Description)
		assert.Contains(v.T(), diagnosis.Description, "Ping:")
		if diagnosis.Status == "failure" {
			assert.NotEmpty(v.T(), diagnosis.Error)
			assert.Contains(v.T(), diagnosis.Metadata, "endpoint")
			assert.Contains(v.T(), diagnosis.Metadata, "raw_error")
		} else {
			assert.Contains(v.T(), diagnosis.Metadata, "endpoint")
		}
	}
}

func (v *inventoryAgentSuite) TestConnectivityWithInvalidProxy() {
	// Configure a non-existent proxy to simulate connectivity failure
	params := agentparams.WithAgentConfig(`
site: datadoghq.com
proxy:
  http: http://invalid-proxy:3128
  https: http://invalid-proxy:3128
inventories_diagnostics_enabled: true
`)
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))

	inventory := getInventoryAgentOutput(v)

	inventoryPayload := unmarshalInventoryAgent(inventory)
	require.NotNil(v.T(), inventoryPayload)

	// Check that diagnostics section exists and contains connectivity checks
	diagnostics := getDiagnosticsFromPayload(inventoryPayload)
	require.NotNil(v.T(), diagnostics, "Diagnostics section should be present")
	require.NotEmpty(v.T(), diagnostics.Connectivity, "Connectivity diagnostics should be present")

	// Check that connectivity diagnostics reflect proxy configuration issues
	hasProxyFailure := false
	for _, diagnosis := range diagnostics.Connectivity {
		assert.NotEmpty(v.T(), diagnosis.Status)
		assert.NotEmpty(v.T(), diagnosis.Description)
		if diagnosis.Status == "failure" {
			hasProxyFailure = true
			assert.NotEmpty(v.T(), diagnosis.Error)
			assert.Contains(v.T(), diagnosis.Metadata, "endpoint")
			assert.Contains(v.T(), diagnosis.Metadata, "raw_error")
		}
	}
	// With invalid proxy, we expect at least some failures
	assert.True(v.T(), hasProxyFailure, "Expected connectivity failures with invalid proxy configuration")
}
