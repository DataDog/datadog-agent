package datastreams

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sync"
)

type kafkaConfig struct {
	Cluster     string `yaml:"cluster" json:"cluster"`
	Topic       string `yaml:"topic" json:"topic"`
	Partition   int32  `yaml:"partition" json:"partition"`
	StartOffset int64  `yaml:"start_offset" json:"start_offset"`
	NMessages   int32  `yaml:"n_messages" json:"n_messages"`
}

type liveMessagesConfig struct {
	Kafka kafkaConfig `yaml:"kafka" json:"kafka"`
	ID    string      `yaml:"id" json:"id"`
}

type Controller struct {
	ac                 autodiscovery.Component
	collectorComponent collector.Component
	m                  sync.RWMutex
	configs            map[string]liveMessagesConfig
	configErrors       map[string]providers.ErrorMsgSet
	configChanges      chan integration.ConfigChanges
}

// String returns the name of the provider.  All Config instances produced
// by this provider will have this value in their Provider field.
func (c *Controller) String() string {
	return "dsm_live_messages"
}

// GetConfigErrors returns a map of errors that occurred on the last Collect
// call, indexed by a description of the resource that generated the error.
// The result is displayed in diagnostic tools such as `agent status`.
func (c *Controller) GetConfigErrors() map[string]providers.ErrorMsgSet {
	return nil
}

type StreamingConfigProvider interface {
	Stream(context.Context) <-chan integration.ConfigChanges
}

func IsConnectedToKafka(ac autodiscovery.Component) bool {
	for _, config := range ac.GetAllConfigs() {
		if config.Name == "kafka_consumer" {
			return true
		}
	}
	return false
}

func NewController(ac autodiscovery.Component, collectorComponent collector.Component) *Controller {
	return &Controller{
		ac:                 ac,
		collectorComponent: collectorComponent,
		configs:            make(map[string]liveMessagesConfig),
		configErrors:       make(map[string]providers.ErrorMsgSet),
		configChanges:      make(chan integration.ConfigChanges, 10),
	}
}

// Stream starts the streaming config provider until the provided
// context is cancelled. Config changes are sent on the return channel.
func (c *Controller) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	// todo[Piotr Wolski] Close gracefully
	return c.configChanges
}

func (c *Controller) getConfigs() []liveMessagesConfig {
	c.m.RLock()
	defer c.m.RUnlock()
	configs := make([]liveMessagesConfig, 0, len(c.configs))
	for _, config := range c.configs {
		configs = append(configs, config)
	}
	return configs
}

// Update updates the config globalStore with the provided updates
func (c *Controller) Update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	fmt.Println("update called!")
	log.Info("Called Update of remote configuration!!!!!!!!!!!!!")
	if err := c.parseRemoteConfig(updates, applyStateCallback); err != nil {
		return
	}
	configChange := integration.ConfigChanges{}
	cfgs := c.ac.GetAllConfigs()
	for _, integrationConfig := range cfgs {
		if integrationConfig.Name != "kafka_consumer" {
			continue
		}
		fmt.Println("integration config is", integrationConfig)
		configChange.Unschedule = append(configChange.Unschedule, integrationConfig)
		updatedConfig := integrationConfig
		updatedConfig.Instances = make([]integration.Data, 0, len(updatedConfig.Instances))
		for _, instance := range integrationConfig.Instances {
			updatedInstance := instance
			p := &updatedInstance
			err := p.SetField("live_messages_configs", c.getConfigs())
			if err != nil {
				log.Error("Error setting field")
			}
			fmt.Println("Finish wrapping: ", string(updatedInstance))
			updatedConfig.Instances = append(updatedConfig.Instances, updatedInstance)
		}
		configChange.Schedule = append(configChange.Schedule, updatedConfig)
	}
	fmt.Println("sent updates to configChanges channel", configChange)
	c.configChanges <- configChange
}

func (c *Controller) parseRemoteConfig(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) error {
	c.m.Lock()
	defer c.m.Unlock()
	for path, rawConfig := range updates {
		// implement delete logic
		var config liveMessagesConfig
		// what is the config ID?
		fmt.Println("config is", string(rawConfig.Config))
		err := json.Unmarshal(rawConfig.Config, &config)
		if err != nil {
			log.Errorf("Can't decode agent configuration provided by remote-config: %v", err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		config.ID = rawConfig.Metadata.ID
		c.configs[path] = config
		fmt.Println("state of path", path, "updated to", config)
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	return nil
}
