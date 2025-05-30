package rcclient

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
	yaml "gopkg.in/yaml.v2"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

var globalStore *atomic.Value

type store struct {
	m     sync.RWMutex
	state map[string]liveMessagesConfig
}

func init() {
	globalStore = &atomic.Value{}
	globalStore.Store(newStore())
}

func newStore() *store {
	return &store{
		state: make(map[string]liveMessagesConfig),
	}
}

type kafkaConfig struct {
	Cluster     string `yaml:"cluster" json:"cluster"`
	Topic       string `yaml:"topic" json:"topic"`
	Partition   int32  `yaml:"partition" json:"partition"`
	StartOffset int64  `yaml:"start_offset" json:"start_offset"`
	NMessages   int32  `yaml:"n_messages" json:"n_messages"`
}

type liveMessagesConfig struct {
	Kafka kafkaConfig `yaml:"kafka" json:"kafka"`
}

type liveMessagesConfigs struct {
	Configs []liveMessagesConfig `yaml:"configs" json:"configs"`
}

// GetYamlConfigs returns the current configs in yaml format
func GetYamlConfigs() ([]byte, error) {
	return yaml.Marshal(globalStore.Load().(*store).getConfigs())
}

// Update updates the config globalStore with the provided updates
func Update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	fmt.Println("update called!")
	globalStore.Load().(*store).update(updates, applyStateCallback)
}

func (s *store) getConfigs() liveMessagesConfigs {
	s.m.RLock()
	defer s.m.RUnlock()
	var configs []liveMessagesConfig
	for _, config := range s.state {
		configs = append(configs, config)
	}
	return liveMessagesConfigs{Configs: configs}
}

func (s *store) update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	s.m.Lock()
	for path, rawConfig := range updates {
		var config liveMessagesConfig
		fmt.Println("config is", string(rawConfig.Config))
		err := json.Unmarshal(rawConfig.Config, &config)
		if err != nil {
			log.Errorf("Can't decode agent configuration provided by remote-config: %v", err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		s.state[path] = config
		fmt.Println("state of path", path, "updated to", config)
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	s.m.Unlock()
	cfg, err := GetYamlConfigs()
	fmt.Println("config updated with", string(cfg), "error", err)
}
