package dynamicinstrumentation

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// Config holds the configuration for the user tracer system probe module
type Config struct {
	ebpf.Config
	DynamicInstrumentationEnabled bool
}

func NewConfig(sysprobeConfig *config.Config) (*Config, error) {
	_, diEnabled := sysprobeConfig.EnabledModules[config.DynamicInstrumentationModule]
	return &Config{
		Config:                        *ebpf.NewConfig(),
		DynamicInstrumentationEnabled: diEnabled,
	}, nil
}
