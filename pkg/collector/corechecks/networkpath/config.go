package networkpath

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"gopkg.in/yaml.v2"
)

// InstanceConfig is used to deserialize integration instance config
type InstanceConfig struct {
	DestName            string `yaml:"name"`
	DestHostname        string `yaml:"hostname"`
	FakeEventMultiplier int    `yaml:"fake_event_multiplier"`
}

type CheckConfig struct {
	DestHostname        string
	DestName            string
	FakeEventMultiplier int
}

// NewCheckConfig builds a new check config
func NewCheckConfig(rawInstance integration.Data, rawInitConfig integration.Data) (*CheckConfig, error) {
	instance := InstanceConfig{}

	err := yaml.Unmarshal(rawInstance, &instance)
	if err != nil {
		return nil, err
	}

	c := &CheckConfig{}

	c.DestHostname = instance.DestHostname
	c.DestName = instance.DestName
	c.FakeEventMultiplier = instance.FakeEventMultiplier

	if c.FakeEventMultiplier == 0 {
		c.FakeEventMultiplier = 1
	}

	return c, nil
}
