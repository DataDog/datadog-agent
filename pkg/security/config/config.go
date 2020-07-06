package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
)

var (
	ErrUnnamedPolicy = errors.New("unnamed policy")
)

type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

type Config struct {
	Debug               bool
	Policies            []Policy
	EnableKernelFilters bool
	SocketPath          string
}

func NewConfig() (*Config, error) {
	c := &Config{
		Debug:               aconfig.Datadog.GetBool("runtime_security_config.debug"),
		EnableKernelFilters: aconfig.Datadog.GetBool("runtime_security_config.enable_kernel_filters"),
		SocketPath:          aconfig.Datadog.GetString("runtime_security_config.socket"),
	}

	policies, ok := aconfig.Datadog.Get("runtime_security_config.policies").([]interface{})
	if !ok {
		return nil, errors.New("policies must be a list of policy definitions")
	}

	for _, p := range policies {
		var policy Policy
		if err := mapstructure.Decode(p, &policy); err != nil {
			return nil, errors.Wrap(err, "invalid policy definition")
		}

		if policy.Name == "" {
			return nil, ErrUnnamedPolicy
		}

		c.Policies = append(c.Policies, policy)
	}

	return c, nil
}
