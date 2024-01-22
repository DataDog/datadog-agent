package netpath

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"gopkg.in/yaml.v2"
)

// InstanceConfig is used to deserialize integration instance config
type InstanceConfig struct {
	DestName     string `yaml:"name"`
	DestHostname string `yaml:"hostname"`
}

type CheckConfig struct {
	DestHostname string
	DestName     string
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

	return c, nil
}
