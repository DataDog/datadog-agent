package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// Policy represents a policy file in the configuration file
type Policy struct {
	Name  string   `mapstructure:"name"`
	Files []string `mapstructure:"files"`
	Tags  []string `mapstructure:"tags"`
}

// Config holds the configuration for the runtime security agent
type Config struct {
	Enabled             bool
	Debug               bool
	Policies            []Policy
	EnableKernelFilters bool
	SocketPath          string
	SyscallMonitor      bool
}

// NewConfig returns a new Config object
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
			return nil, errors.New("unnamed policy")
		}

		c.Policies = append(c.Policies, policy)
	}

	return c, nil
}
