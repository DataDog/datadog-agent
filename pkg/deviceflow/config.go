// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package deviceflow

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// IsEnabled returns whether SNMP trap collection is enabled in the Agent configuration.
func IsEnabled() bool {
	// TODO: add netflow_enabled to config.go
	return config.Datadog.GetBool("device_flow.enabled")
}

// Config contains configuration for SNMP trap listeners.
// YAML field tags provided for test marshalling purposes.
type Config struct {
	Port        uint16 `mapstructure:"port" yaml:"port"`
	BindHost    string `mapstructure:"bind_host" yaml:"bind_host"`
	StopTimeout int    `mapstructure:"stop_timeout" yaml:"stop_timeout"`
	SendEvents  bool   `mapstructure:"send_events" yaml:"send_events"`
	SendMetrics bool   `mapstructure:"send_metrics" yaml:"send_metrics"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig() (*Config, error) {
	var c Config
	err := config.Datadog.UnmarshalKey("device_flow", &c)
	if err != nil {
		return nil, err
	}

	// Set defaults.
	if c.Port == 0 {
		c.Port = defaultPortNETFLOW
	}
	if c.BindHost == "" {
		// Default to global bind_host option.
		c.BindHost = config.GetBindHost()
	}
	if c.StopTimeout == 0 {
		c.StopTimeout = defaultStopTimeout
	}

	return &c, nil
}

// Addr returns the host:port address to listen on.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.BindHost, c.Port)
}
