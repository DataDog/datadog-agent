// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package datastreams

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	noopautoconfig "github.com/DataDog/datadog-agent/comp/core/autodiscovery/noopimpl"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockedAutodiscoveryActions struct {
	autodiscovery.Component
	configs []integration.Config
}

func getMockedAutodiscoveryActions(t *testing.T, configs []integration.Config) autodiscovery.Component {
	return &mockedAutodiscoveryActions{
		Component: fxutil.Test[autodiscovery.Component](
			t,
			noopautoconfig.Module(),
		),
		configs: configs,
	}
}

func (m *mockedAutodiscoveryActions) GetUnresolvedConfigs() []integration.Config {
	return m.configs
}

func (m *mockedAutodiscoveryActions) GetAllConfigs() []integration.Config {
	return m.configs
}

const kafkaConsumerConfig = `
kafka_connect_str: localhost:9092
consumer_groups:
  - test-group
topics:
  - test-topic
sasl_mechanism: PLAIN
sasl_plain_username: user
sasl_plain_password: pass
security_protocol: SASL_SSL
`

func TestActionsController(t *testing.T) {
	kafkaConsumerCfg := integration.Config{
		Name:       kafkaConsumerIntegrationName,
		Instances:  []integration.Data{integration.Data(kafkaConsumerConfig)},
		InitConfig: integration.Data{},
	}

	c := &actionsController{
		ac:            getMockedAutodiscoveryActions(t, []integration.Config{kafkaConsumerCfg}),
		rcclient:      &mockedRcClient{},
		configChanges: make(chan integration.ConfigChanges, 10),
	}

	// Create a kafka actions config
	actions := map[string]any{
		"read_messages": map[string]any{
			"topic":      "test-topic",
			"partition":  0,
			"offset":     100,
			"n_messages": 10,
		},
	}
	actionsJSON, err := json.Marshal(actions)
	require.NoError(t, err)

	kafkaActionsConfig := kafkaActionsConfig{
		Actions:          actionsJSON,
		BootstrapServers: "localhost:9092",
	}
	serializedConfig, err := json.Marshal(kafkaActionsConfig)
	require.NoError(t, err)

	rcUpdate := map[string]state.RawConfig{
		"config_1": {Config: []byte("invalid")},
		"config_2": {Config: serializedConfig, Metadata: state.Metadata{ID: "test_remote_config_id"}},
	}

	updateStatus := make(map[string]state.ApplyStatus)
	callback := func(path string, status state.ApplyStatus) {
		updateStatus[path] = status
	}

	c.update(rcUpdate, callback)

	// Verify error handling for invalid config
	assert.Equal(t, state.ApplyStateError, updateStatus["config_1"].State)
	assert.Contains(t, updateStatus["config_1"].Error, "invalid character")

	// Verify successful config
	assert.Equal(t, state.ApplyStateAcknowledged, updateStatus["config_2"].State)

	// Check the scheduled config
	updates := c.Stream(context.Background())
	cfg := <-updates

	require.Len(t, cfg.Schedule, 1)
	assert.Empty(t, cfg.Unschedule)

	scheduledCfg := cfg.Schedule[0]
	assert.Equal(t, kafkaActionsIntegrationName, scheduledCfg.Name)
	require.Len(t, scheduledCfg.Instances, 1)

	// Verify the instance has auth merged in
	var instance map[string]any
	err = yaml.Unmarshal(scheduledCfg.Instances[0], &instance)
	require.NoError(t, err)

	// Check that auth fields are present
	assert.Equal(t, "localhost:9092", instance["kafka_connect_str"])
	assert.Equal(t, "PLAIN", instance["sasl_mechanism"])
	assert.Equal(t, "user", instance["sasl_plain_username"])
	assert.Equal(t, "pass", instance["sasl_plain_password"])
	assert.Equal(t, "SASL_SSL", instance["security_protocol"])

	assert.Equal(t, true, instance["run_once"])

	// Check that remote_config_id was injected
	assert.Equal(t, "test_remote_config_id", instance["remote_config_id"])

	// Check that actions are present
	assert.NotNil(t, instance["read_messages"])
}

func TestActionsControllerNoBootstrapServers(t *testing.T) {
	kafkaConsumerCfg := integration.Config{
		Name:       kafkaConsumerIntegrationName,
		Instances:  []integration.Data{integration.Data(kafkaConsumerConfig)},
		InitConfig: integration.Data{},
	}

	c := &actionsController{
		ac:            getMockedAutodiscoveryActions(t, []integration.Config{kafkaConsumerCfg}),
		rcclient:      &mockedRcClient{},
		configChanges: make(chan integration.ConfigChanges, 10),
	}

	// Create config without bootstrap_servers - should match first kafka_consumer
	actions := map[string]any{
		"read_messages": map[string]any{
			"topic": "test-topic",
		},
	}
	actionsJSON, err := json.Marshal(actions)
	require.NoError(t, err)

	kafkaActionsConfig := kafkaActionsConfig{
		Actions: actionsJSON,
		// No BootstrapServers specified
	}
	serializedConfig, err := json.Marshal(kafkaActionsConfig)
	require.NoError(t, err)

	rcUpdate := map[string]state.RawConfig{
		"config_1": {Config: serializedConfig, Metadata: state.Metadata{ID: "no_bootstrap_test_id"}},
	}

	updateStatus := make(map[string]state.ApplyStatus)
	callback := func(path string, status state.ApplyStatus) {
		updateStatus[path] = status
	}

	c.update(rcUpdate, callback)

	// Should succeed by matching first kafka_consumer
	assert.Equal(t, state.ApplyStateAcknowledged, updateStatus["config_1"].State)
}

func TestActionsControllerNoMatchingKafkaConsumer(t *testing.T) {
	c := &actionsController{
		ac:            getMockedAutodiscoveryActions(t, []integration.Config{}),
		rcclient:      &mockedRcClient{},
		configChanges: make(chan integration.ConfigChanges, 10),
	}

	actions := map[string]any{
		"read_messages": map[string]any{
			"topic": "test-topic",
		},
	}
	actionsJSON, err := json.Marshal(actions)
	require.NoError(t, err)

	kafkaActionsConfig := kafkaActionsConfig{
		Actions:          actionsJSON,
		BootstrapServers: "localhost:9092",
	}
	serializedConfig, err := json.Marshal(kafkaActionsConfig)
	require.NoError(t, err)

	rcUpdate := map[string]state.RawConfig{
		"config_1": {Config: serializedConfig, Metadata: state.Metadata{ID: "no_match_test_id"}},
	}

	updateStatus := make(map[string]state.ApplyStatus)
	callback := func(path string, status state.ApplyStatus) {
		updateStatus[path] = status
	}

	c.update(rcUpdate, callback)

	// Should fail with error
	assert.Equal(t, state.ApplyStateError, updateStatus["config_1"].State)
	assert.Contains(t, updateStatus["config_1"].Error, "kafka_consumer integration")
}
