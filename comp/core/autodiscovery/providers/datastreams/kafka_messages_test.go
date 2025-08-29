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

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
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

type mockedAutodiscovery struct {
	autodiscovery.Component
}

func getMockedAutodiscovery(t *testing.T) autodiscovery.Component {
	return &mockedAutodiscovery{
		fxutil.Test[autodiscovery.Component](
			t,
			noopautoconfig.Module(),
		),
	}
}

const initialConfig = `
kafka_connect_str: localhost:9092
consumer_groups:
  my-consumer-group:
    marvel: [0]
topics:
  - my-topic
tags:
  - env:dev
`

// keys are sorted in the modified config
const modifiedConfig = `consumer_groups:
  my-consumer-group:
    marvel:
    - 0
kafka_connect_str: localhost:9092
live_messages_configs:
- kafka:
    cluster: test-cluster
    topic: test-topic
    partition: 1
    start_offset: 34
    n_messages: 10
    value_format: avro
    value_schema: |
      {"type":"record","name":"User","namespace":"com.example","fields":[{"name":"id","type":"int"},{"name":"name","type":"string"},{"name":"email","type":["null","string"],"default":null}]}
    value_uses_schema_registry: true
    key_format: string
    key_schema: ""
    key_uses_schema_registry: false
  id: config_2_id
tags:
- env:dev
topics:
- my-topic
`

func (m *mockedAutodiscovery) GetUnresolvedConfigs() []integration.Config {
	return []integration.Config{{
		Name:       kafkaConsumerIntegrationName,
		Instances:  []integration.Data{integration.Data(initialConfig)},
		InitConfig: integration.Data{},
	}}
}
func (m *mockedAutodiscovery) GetAllConfigs() []integration.Config {
	return []integration.Config{{
		Name:      kafkaConsumerIntegrationName,
		Instances: []integration.Data{integration.Data(initialConfig)},
	}}
}

func TestController(t *testing.T) {
	c := &controller{
		ac:            getMockedAutodiscovery(t),
		rcclient:      &mockedRcClient{},
		configChanges: make(chan integration.ConfigChanges, 10),
	}
	originalCfg := integration.Config{
		Name:       kafkaConsumerIntegrationName,
		Instances:  []integration.Data{integration.Data(initialConfig)},
		InitConfig: integration.Data{},
	}
	config := liveMessagesConfig{
		ID: "config_2_id",
		Kafka: kafkaConfig{
			Cluster:                 "test-cluster",
			Topic:                   "test-topic",
			Partition:               1,
			StartOffset:             34,
			NMessages:               10,
			ValueFormat:             "avro",
			ValueSchema:             "{\"type\":\"record\",\"name\":\"User\",\"namespace\":\"com.example\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"name\",\"type\":\"string\"},{\"name\":\"email\",\"type\":[\"null\",\"string\"],\"default\":null}]}\n",
			ValueUsesSchemaRegistry: true,
			KeyFormat:               "string",
		},
	}
	serializedConfig, err := json.Marshal(config)
	assert.Nil(t, err)
	rcUpdate := map[string]state.RawConfig{
		"config_1": {Config: []byte("invalid")},
		"config_2": {Config: serializedConfig, Metadata: state.Metadata{ID: "config_2_id"}},
	}
	updateStatus := make(map[string]state.ApplyStatus)
	callback := func(path string, status state.ApplyStatus) {
		updateStatus[path] = status
	}
	c.update(rcUpdate, callback)
	assert.Equal(t, map[string]state.ApplyStatus{
		"config_1": {State: state.ApplyStateError, Error: "invalid character 'i' looking for beginning of value"},
		"config_2": {State: state.ApplyStateAcknowledged},
	}, updateStatus)
	updates := c.Stream(context.Background())
	cfg := <-updates
	updatedConfig := integration.Config{
		Name:       kafkaConsumerIntegrationName,
		Instances:  []integration.Data{integration.Data(modifiedConfig)},
		InitConfig: integration.Data{},
		LogsConfig: integration.Data(logsConfig),
	}
	assert.Len(t, cfg.Schedule, 1)
	assert.Len(t, cfg.Unschedule, 1)
	assert.Equal(t, originalCfg, cfg.Unschedule[0])
	assert.Equal(t, updatedConfig, cfg.Schedule[0])
}
