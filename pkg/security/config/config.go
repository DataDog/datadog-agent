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
	Enabled             bool
	Debug               bool
	Policies            []Policy
	EnableKernelFilters bool
	SocketPath          string
	SyscallMonitor      bool
}

func NewConfig() (*Config, error) {
	c := &Config{
		Enabled:             aconfig.Datadog.GetBool("runtime_security_config.enabled"),
		Debug:               aconfig.Datadog.GetBool("runtime_security_config.debug"),
		EnableKernelFilters: aconfig.Datadog.GetBool("runtime_security_config.enable_kernel_filters"),
		SocketPath:          aconfig.Datadog.GetString("runtime_security_config.socket"),
		SyscallMonitor:      aconfig.Datadog.GetBool("runtime_security_config.syscall_monitor.enabled"),
	}

	if !c.Enabled {
		return c, nil
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
