package rcclient

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
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
}

func NewController(ac autodiscovery.Component, collectorComponent collector.Component) *Controller {
	return &Controller{
		ac:                 ac,
		collectorComponent: collectorComponent,
		configs:            make(map[string]liveMessagesConfig),
	}
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

type liveMessagesCheck struct {
	check.Check
	instanceConfig string
}

func (l *liveMessagesCheck) InstanceConfig() string {
	return l.instanceConfig
}

func wrapLiveMessageCheck(c *Controller, kafkaCheck check.Check) (check.Check, error) {
	data := integration.Data(kafkaCheck.InstanceConfig())
	err := data.SetField("live_messages_configs", c.getConfigs())
	if err != nil {
		return nil, fmt.Errorf("failed to set live messages configs: %w", err)
	}
	return &liveMessagesCheck{
		Check:          kafkaCheck,
		instanceConfig: string(data),
	}, nil
}

// Update updates the config globalStore with the provided updates
func (c *Controller) Update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	fmt.Println("update called!")
	if err := c.parseRemoteConfig(updates, applyStateCallback); err != nil {
		return
	}
	checks := pkgcollector.GetChecksByNameForConfigs("kafka_consumer", c.ac.GetAllConfigs())
	updatedChecks := make([]check.Check, 0, len(checks))
	var err error
	for _, check := range checks {
		fmt.Println("check is", check)
		fmt.Println("config is", check.InstanceConfig())
		fmt.Println("init config is", check.InitConfig())
		liveMessageCheck, err := wrapLiveMessageCheck(c, check)
		if err != nil {
			log.Errorf("Failed to wrap live message check: %v. Using original check", err)
			updatedChecks = append(updatedChecks, check)
		}
		updatedChecks = append(updatedChecks, liveMessageCheck)
	}
	_, err = c.collectorComponent.ReloadAllCheckInstances("kafka_consumer", checks)
	if err != nil {
		log.Errorf("Failed to reload checks for kafka_consumer: %v", err)
		return
	}
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
