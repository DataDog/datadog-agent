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
	state map[string]kafkaConfig
}

func init() {
	globalStore = &atomic.Value{}
	globalStore.Store(newStore())
}

func newStore() *store {
	return &store{
		state: make(map[string]kafkaConfig),
	}
}

type kafkaConfig struct {
	Action    string `yaml:"action" json:"action"`
	Topic     string `yaml:"topic" json:"topic"`
	Partition int32  `yaml:"partition" json:"partition"`
	Offset    int64  `yaml:"offset" json:"offset"`
	Key       string `yaml:"key" json:"key"`
}

type kafkaConfigs struct {
	Configs []kafkaConfig `yaml:"configs" json:"configs"`
}

// GetYamlConfigs returns the current configs in yaml format
func GetYamlConfigs() ([]byte, error) {
	return yaml.Marshal(globalStore.Load().(*store).getConfigs())
}

// Update updates the config globalStore with the provided updates
func Update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	globalStore.Load().(*store).update(updates, applyStateCallback)
}

func (s *store) getConfigs() kafkaConfigs {
	s.m.RLock()
	defer s.m.RUnlock()
	var configs []kafkaConfig
	for _, config := range s.state {
		configs = append(configs, config)
	}
	return kafkaConfigs{Configs: configs}
}

func (s *store) update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	s.m.Lock()
	for path, rawConfig := range updates {
		var config kafkaConfig
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
