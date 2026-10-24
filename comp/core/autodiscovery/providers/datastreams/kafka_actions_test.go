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
	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	noopautoconfig "github.com/DataDog/datadog-agent/comp/core/autodiscovery/noopimpl"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockedRcClient struct{}

func (m *mockedRcClient) SubscribeAgentTask() {}

func (m *mockedRcClient) Subscribe(data.Product, func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

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
schema_registry_url: http://localhost:8081
schema_registry_username: sr-user
schema_registry_password: sr-pass
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

	// Compared as text so any change in serialization fails here, including
	// style the Python check would tolerate. consumer_groups and topics are
	// absent because they are not on the auth allowlist.
	expected := `kafka_connect_str: localhost:9092
read_messages:
    n_messages: 10
    offset: 100
    partition: 0
    topic: test-topic
remote_config_id: test_remote_config_id
run_once: true
sasl_mechanism: PLAIN
sasl_plain_password: pass
sasl_plain_username: user
schema_registry_password: sr-pass
schema_registry_url: http://localhost:8081
schema_registry_username: sr-user
security_protocol: SASL_SSL
`
	assert.Equal(t, expected, string(scheduledCfg.Instances[0]))
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

func TestNormalizeBootstrapServers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "comma-separated list with hyphens and uppercase",
			input:    "Kafka-Broker1:9092,kafka-broker2:9092",
			expected: "kafka_broker1:9092_kafka_broker2:9092",
		},
		{
			name:     "multiple special chars collapsed and dot-underscore cleaned",
			input:    "kafka.-broker:9092,  broker2:9092",
			expected: "kafka.broker:9092_broker2:9092",
		},
		{
			name:     "leading and trailing special chars trimmed",
			input:    "-broker.example.com:9092-",
			expected: "broker.example.com:9092",
		},
		{
			name:  "long broker list truncated to backend tag limit",
			input: "b-1.demosumuseast1main.l4fk14.c6.kafka.us-east-1.amazonaws.com:9096,b-2.demosumuseast1main.l4fk14.c6.kafka.us-east-1.amazonaws.com:9096,b-3.demosumuseast1main.l4fk14.c6.kafka.us-east-1.amazonaws.com:9096",
			// Full normalized value would be 202 chars, but the backend truncates
			// the tag "bootstrap_servers:<value>" to 200 chars total, leaving
			// 200 - len("bootstrap_servers:") = 182 chars for the value.
			expected: "b_1.demosumuseast1main.l4fk14.c6.kafka.us_east_1.amazonaws.com:9096_b_2.demosumuseast1main.l4fk14.c6.kafka.us_east_1.amazonaws.com:9096_b_3.demosumuseast1main.l4fk14.c6.kafka.us_east",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizeBootstrapServers(tt.input))
		})
	}
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

func TestActionsControllerKeepsNumericTypes(t *testing.T) {
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

	// start_offset is above 2^53 so a float64 round-trip would lose precision.
	actionsJSON := []byte(`{
		"read_messages": {
			"topic": "test-topic",
			"partition": -1,
			"start_offset": 9007199254740993,
			"n_messages_retrieved": 10,
			"max_scanned_messages": 1000,
			"timeout_ms": 5000,
			"ratio": 0.5
		},
		"update_consumer_group_offsets": {
			"consumer_group": "test-group",
			"offsets": [{"topic": "test-topic", "partition": 3, "offset": 12345}]
		}
	}`)

	serializedConfig, err := json.Marshal(kafkaActionsConfig{
		Actions:          actionsJSON,
		BootstrapServers: "localhost:9092",
	})
	require.NoError(t, err)

	rcUpdate := map[string]state.RawConfig{
		"config_1": {Config: serializedConfig, Metadata: state.Metadata{ID: "numeric_types_test_id"}},
	}
	updateStatus := make(map[string]state.ApplyStatus)
	c.update(rcUpdate, func(path string, status state.ApplyStatus) {
		updateStatus[path] = status
	})
	require.Equal(t, state.ApplyStateAcknowledged, updateStatus["config_1"].State)

	cfg := <-c.Stream(context.Background())
	require.Len(t, cfg.Schedule, 1)
	require.Len(t, cfg.Schedule[0].Instances, 1)

	var instance map[string]any
	require.NoError(t, yaml.Unmarshal(cfg.Schedule[0].Instances[0], &instance))

	readMessages, ok := instance["read_messages"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-topic", readMessages["topic"])
	assert.Equal(t, -1, readMessages["partition"])
	assert.Equal(t, 9007199254740993, readMessages["start_offset"])
	assert.Equal(t, 10, readMessages["n_messages_retrieved"])
	assert.Equal(t, 1000, readMessages["max_scanned_messages"])
	assert.Equal(t, 5000, readMessages["timeout_ms"])
	assert.Equal(t, 0.5, readMessages["ratio"])

	updateOffsets, ok := instance["update_consumer_group_offsets"].(map[string]any)
	require.True(t, ok)
	offsets, ok := updateOffsets["offsets"].([]any)
	require.True(t, ok)
	require.Len(t, offsets, 1)
	offset, ok := offsets[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-topic", offset["topic"])
	assert.Equal(t, 3, offset["partition"])
	assert.Equal(t, 12345, offset["offset"])
}
