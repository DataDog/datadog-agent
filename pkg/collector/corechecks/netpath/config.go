package netpath

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"gopkg.in/yaml.v2"
)

// InstanceConfig is used to deserialize integration instance config
type InstanceConfig struct {
	Hostname      string `yaml:"hostname"`
	Numpaths      uint16 `yaml:"numpaths"`
	TargetService string `yaml:"target_service"`
}

type CheckConfig struct {
	Hostname      string
	Numpaths      uint16
	TargetService string
}

// NewCheckConfig builds a new check config
func NewCheckConfig(rawInstance integration.Data, rawInitConfig integration.Data) (*CheckConfig, error) {
	instance := InstanceConfig{}

	err := yaml.Unmarshal(rawInstance, &instance)
	if err != nil {
		return nil, err
	}

	c := &CheckConfig{}

	c.Hostname = instance.Hostname
	c.TargetService = instance.TargetService
	c.Numpaths = instance.Numpaths

	return c, nil
}
