// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

// Package datastreams contains logic to configure the kafka_consumer integration via remote configuration to fetch messages from Kafka
package datastreams

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kafkaConsumerIntegrationName = "kafka_consumer"
	logsConfig                   = "[{\"type\":\"integration\",\"service\":\"kafka_consumer\",\"source\":\"kafka_consumer\"}]"
)

type kafkaConfig struct {
	Cluster                 string `yaml:"cluster" json:"cluster"`
	Topic                   string `yaml:"topic" json:"topic"`
	Partition               int32  `yaml:"partition" json:"partition"`
	StartOffset             int64  `yaml:"start_offset" json:"start_offset"`
	NMessages               int32  `yaml:"n_messages" json:"n_messages"`
	ValueFormat             string `yaml:"value_format" json:"value_format"`
	ValueSchema             string `yaml:"value_schema" json:"value_schema"`
	ValueUsesSchemaRegistry bool   `yaml:"value_uses_schema_registry" json:"value_uses_schema_registry"`
	KeyFormat               string `yaml:"key_format" json:"key_format"`
	KeySchema               string `yaml:"key_schema" json:"key_schema"`
	KeyUsesSchemaRegistry   bool   `yaml:"key_uses_schema_registry" json:"key_uses_schema_registry"`
}

type liveMessagesConfig struct {
	Kafka kafkaConfig `yaml:"kafka" json:"kafka"`
	ID    string      `yaml:"id" json:"id"`
}

// controller listens to remote configuration updates for the Data Streams live messages feature
// and configures the kafka_consumer integration to fetch messages from Kafka.
type controller struct {
	ac            autodiscovery.Component
	rcclient      rcclient.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool
}

// String returns the name of the provider.  All Config instances produced
// by this provider will have this value in their Provider field.
func (c *controller) String() string {
	return names.DataStreamsLiveMessages
}

// GetConfigErrors returns a map of errors that occurred on the last Collect
// call, indexed by a description of the resource that generated the error.
// The result is displayed in diagnostic tools such as `agent status`.
func (c *controller) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

// manageSubscriptionToRC subscribes to remote configuration updates if the agent is running the kafka_consumer integration.
func (c *controller) manageSubscriptionToRC() {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()
	for range ticker.C {
		c.closeMutex.RLock()
		if c.closed {
			c.closeMutex.RUnlock()
			return
		}
		c.closeMutex.RUnlock()
		if isConnectedToKafka(c.ac) {
			c.rcclient.Subscribe(data.ProductDataStreamsLiveMessages, c.update)
			return
		}
	}
}

func isConnectedToKafka(ac autodiscovery.Component) bool {
	for _, config := range ac.GetAllConfigs() {
		if config.Name == kafkaConsumerIntegrationName {
			return true
		}
	}
	return false
}

// NewController creates a new controller instance
func NewController(ac autodiscovery.Component, rcclient rcclient.Component) types.ConfigProvider {
	c := &controller{
		ac:            ac,
		rcclient:      rcclient,
		configChanges: make(chan integration.ConfigChanges, 10),
	}
	// Send an empty config change to ensure that config_poller starts correctly
	c.configChanges <- integration.ConfigChanges{}
	go c.manageSubscriptionToRC()
	return c
}

// Stream starts sending configuration updates for the kafka_consumer integration to the output channel.
func (c *controller) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	go func() {
		<-ctx.Done()
		c.closeMutex.Lock()
		defer c.closeMutex.Unlock()
		if c.closed {
			return
		}
		c.closed = true
		close(c.configChanges)
	}()
	return c.configChanges
}

// update parses updates from remote configuration, and configures the kafka_consumer integration to fetch messages from Kafka
func (c *controller) update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	remoteConfigs := parseRemoteConfig(updates, applyStateCallback)
	if len(remoteConfigs) == 0 {
		return
	}
	configChange := integration.ConfigChanges{}
	cfgs := c.ac.GetAllConfigs()
	for _, integrationConfig := range cfgs {
		if integrationConfig.Name != kafkaConsumerIntegrationName {
			continue
		}
		configChange.Unschedule = append(configChange.Unschedule, integrationConfig)
		updatedConfig := integrationConfig
		updatedConfig.Instances = make([]integration.Data, 0, len(updatedConfig.Instances))
		if updatedConfig.LogsConfig == nil {
			updatedConfig.LogsConfig = integration.Data(logsConfig)
		}
		for _, instance := range integrationConfig.Instances {
			updatedInstance := instance
			p := &updatedInstance
			err := p.SetField("live_messages_configs", remoteConfigs)
			if err != nil {
				log.Error("Live messages update: Error setting field")
			}
			updatedConfig.Instances = append(updatedConfig.Instances, updatedInstance)
		}
		configChange.Schedule = append(configChange.Schedule, updatedConfig)
	}
	if len(configChange.Schedule) == 0 && len(configChange.Unschedule) == 0 {
		return
	}
	c.closeMutex.RLock()
	defer c.closeMutex.RUnlock()
	if c.closed {
		return
	}
	c.configChanges <- configChange
}

func parseRemoteConfig(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) (configs []liveMessagesConfig) {
	for path, rawConfig := range updates {
		var config liveMessagesConfig
		err := json.Unmarshal(rawConfig.Config, &config)
		if err != nil {
			log.Errorf("Can't decode data streams live messages configuration provided by remote-config: %v", err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		config.ID = rawConfig.Metadata.ID
		configs = append(configs, config)
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	return configs
}
