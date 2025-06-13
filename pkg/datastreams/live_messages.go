package datastreams

import (
	"context"
	"encoding/json"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sync"
	"time"
)

const (
	kafkaConsumerIntegrationName = "kafka_consumer"
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
	ac            autodiscovery.Component
	configChanges chan integration.ConfigChanges
	closeMutex    sync.RWMutex
	closed        bool
}

// String returns the name of the provider.  All Config instances produced
// by this provider will have this value in their Provider field.
func (c *Controller) String() string {
	return names.DataStreamsLiveMessages
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

func (c *Controller) ManageSubscriptionToRC(subscribe func()) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.closeMutex.RLock()
		if c.closed {
			c.closeMutex.RUnlock()
			return
		}
		c.closeMutex.RUnlock()
		if IsConnectedToKafka(c.ac) {
			subscribe()
			return
		}
	}
}

func IsConnectedToKafka(ac autodiscovery.Component) bool {
	for _, config := range ac.GetAllConfigs() {
		if config.Name == kafkaConsumerIntegrationName {
			return true
		}
	}
	return false
}

func NewController(ac autodiscovery.Component) *Controller {
	return &Controller{
		ac:            ac,
		configChanges: make(chan integration.ConfigChanges, 10),
	}
}

// Stream starts the streaming config provider until the provided
// context is cancelled. Config changes are sent on the return channel.
func (c *Controller) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	go func() {
		<-ctx.Done()
		c.closeMutex.Lock()
		defer c.closeMutex.Unlock()
		c.closed = true
		close(c.configChanges)
	}()
	return c.configChanges
}

// Update parses updates from remote configuration, and configures the kafka_consumer integration to fetch messages from Kafka
func (c *Controller) Update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
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
			updatedConfig.LogsConfig = integration.Data("[{\"type\":\"integration\",\"service\":\"kafka_consumer\",\"source\":\"kafka_consumer\"}]")
		}
		for _, instance := range integrationConfig.Instances {
			updatedInstance := instance
			p := &updatedInstance
			err := p.SetField("live_messages_configs", remoteConfigs)
			if err != nil {
				log.Error("Error setting field")
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
