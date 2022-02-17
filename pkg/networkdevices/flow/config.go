// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package flow

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// IsEnabled returns whether SNMP trap collection is enabled in the Agent configuration.
func IsEnabled() bool {
	// TODO: add netflow_enabled to config.go
	return config.Datadog.GetBool("network_devices.flow.enabled")
}

// Config contains configuration for SNMP trap listeners.
// YAML field tags provided for test marshalling purposes.
type Config struct {
	configs     []ListenerConfig `mapstructure:"configs" yaml:"configs"`
	StopTimeout int              `mapstructure:"stop_timeout" yaml:"stop_timeout"`
}

// ListenerConfig contains configuration for a single flow listener
type ListenerConfig struct {
	// TODO: Need both mapstructure and yaml ?
	FlowType FlowType `mapstructure:"flow_type" yaml:"flow_type"`
	Port     uint16   `mapstructure:"port" yaml:"port"`
	BindHost string   `mapstructure:"bind_host" yaml:"bind_host"`

	// TODO: remove after dev stage
	SendEvents  bool `mapstructure:"send_events" yaml:"send_events"`
	SendMetrics bool `mapstructure:"send_metrics" yaml:"send_metrics"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig() (*Config, error) {
	var c Config
	err := config.Datadog.UnmarshalKey("network_devices.flow", &c)
	if err != nil {
		return nil, err
	}

	// TODO: Set default Port per Flow Type
	//       defaultPortNETFLOW = uint16(2055)
	//		 defaultPortIPFIX   = uint16(4739)
	//		 defaultPortSFLOW   = uint16(6343)
	// Set defaults.
	//if c.Port == 0 {
	//	c.Port = defaultPortNETFLOW
	//}
	// TODO: When bindHost is needed?
	//if c.BindHost == "" {
	//	// Default to global bind_host option.
	//	c.BindHost = config.GetBindHost()
	//}
	if c.StopTimeout == 0 {
		c.StopTimeout = defaultStopTimeout
	}

	return &c, nil
}

// Addr returns the host:port address to listen on.
func (c *ListenerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.BindHost, c.Port)
}
