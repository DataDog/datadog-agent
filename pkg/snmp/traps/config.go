// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/gosnmp/gosnmp"
)

// IsEnabled returns whether SNMP trap collection is enabled in the Agent configuration.
func IsEnabled() bool {
	return config.Datadog.GetBool("snmp_traps_enabled")
}

// Config contains configuration for SNMP trap listeners.
// YAML field tags provided for test marshalling purposes.
type Config struct {
	Port             uint16   `mapstructure:"port" yaml:"port"`
	CommunityStrings []string `mapstructure:"community_strings" yaml:"community_strings"`
	BindHost         string   `mapstructure:"bind_host" yaml:"bind_host"`
	StopTimeout      int      `mapstructure:"stop_timeout" yaml:"stop_timeout"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig() (*Config, error) {
	var c Config
	err := config.Datadog.UnmarshalKey("snmp_traps_config", &c)
	if err != nil {
		return nil, err
	}

	// Validate required fields.
	if c.CommunityStrings == nil || len(c.CommunityStrings) == 0 {
		return nil, errors.New("`community_strings` is required and must be non-empty")
	}

	// Set defaults.
	if c.Port == 0 {
		c.Port = defaultPort
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

// BuildV2Params returns a valid GoSNMP SNMPv2 params structure from configuration.
func (c *Config) BuildV2Params() *gosnmp.GoSNMP {
	return &gosnmp.GoSNMP{
		Port:      c.Port,
		Transport: "udp",
		Version:   gosnmp.Version2c,
		Logger:    &trapLogger{},
	}
}
