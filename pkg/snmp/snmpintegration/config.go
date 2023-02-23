// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package snmpintegration

// InterfaceConfig interface related configs (e.g. interface speed override)
type InterfaceConfig struct {
	MatchField string `mapstructure:"match_field" yaml:"match_field" json:"match_field"` // e.g. name, index
	MatchValue string `mapstructure:"match_value" yaml:"match_value" json:"match_value"` // e.g. eth0 (name), 10 (index)
	InSpeed    uint64 `mapstructure:"in_speed" yaml:"in_speed" json:"in_speed"`          // inbound speed override in bps
	OutSpeed   uint64 `mapstructure:"out_speed" yaml:"out_speed" json:"out_speed"`       // outbound speed override in bps
}
