// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package snmpintegration

import (
	"encoding/json"
	"fmt"
	"time"
)

// InterfaceConfig interface related configs (e.g. interface speed override)
type InterfaceConfig struct {
	MatchField string   `mapstructure:"match_field" yaml:"match_field" json:"match_field"` // e.g. name, index
	MatchValue string   `mapstructure:"match_value" yaml:"match_value" json:"match_value"` // e.g. eth0 (name), 10 (index)
	InSpeed    uint64   `mapstructure:"in_speed" yaml:"in_speed" json:"in_speed"`          // inbound speed override in bps
	OutSpeed   uint64   `mapstructure:"out_speed" yaml:"out_speed" json:"out_speed"`       // outbound speed override in bps
	Tags       []string `mapstructure:"tags" yaml:"tags" json:"tags"`                      // interface tags
}

// PingConfig encapsulates the configuration for ping
type PingConfig struct {
	Linux    PingLinuxConfig `mapstructure:"linux" yaml:"linux" json:"linux"`
	Enabled  *bool           `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Interval *int            `mapstructure:"interval" yaml:"interval" json:"interval"`
	Timeout  *int            `mapstructure:"timeout" yaml:"timeout" json:"timeout"`
	Count    *int            `mapstructure:"count" yaml:"count" json:"count"`
}

type PingLinuxConfig struct {
	UseRawSocket *bool `mapstructure:"use_raw_socket" yaml:"use_raw_socket" json:"use_raw_socket"`
}

type PackedPingConfig PingConfig

func (pc *PackedPingConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var pingCfg PingConfig
	err := unmarshal(&pingCfg)
	// Needed for autodiscovery case where the passed config will be a string
	if err != nil {
		var pingCfgJSON string
		if err = unmarshal(&pingCfgJSON); err != nil {
			return fmt.Errorf("cannot unmarshal to string: %s", err)
		}
		if pingCfgJSON == "" {
			return nil
		}
		if err = json.Unmarshal([]byte(pingCfgJSON), &pingCfg); err != nil {
			return fmt.Errorf("cannot unmarshal json to snmpintegration.PingConfig: %s", err)
		}
	}

	*pc = PackedPingConfig(pingCfg)
	return nil
}

type RDNSConfig struct {
	Enabled bool          `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Timeout time.Duration `mapstructure:"timeout" yaml:"timeout" json:"timeout"`
}
