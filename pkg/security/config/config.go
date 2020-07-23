package config

import (
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
	PoliciesDir         string
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
		PoliciesDir:         aconfig.Datadog.GetString("runtime_security_config.policies.dir"),
	}

	return c, nil
}
