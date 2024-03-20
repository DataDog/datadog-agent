// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package networkpath

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"gopkg.in/yaml.v2"
)

// InstanceConfig is used to deserialize integration instance config
type InstanceConfig struct {
	DestHostname string `yaml:"hostname"`

	DestPort uint16 `yaml:"port"`

	MaxTTL uint8 `yaml:"max_ttl"`

	TimeoutMs uint `yaml:"timeout"` // millisecond
}

// CheckConfig defines the configuration of the
// Network Path integration
type CheckConfig struct {
	DestHostname string
	DestPort     uint16
	MaxTTL       uint8
	TimeoutMs    uint
}

// NewCheckConfig builds a new check config
func NewCheckConfig(rawInstance integration.Data, _ integration.Data) (*CheckConfig, error) {
	instance := InstanceConfig{}

	err := yaml.Unmarshal(rawInstance, &instance)
	if err != nil {
		return nil, err
	}

	c := &CheckConfig{}

	c.DestHostname = instance.DestHostname
	c.DestPort = instance.DestPort
	c.MaxTTL = instance.MaxTTL
	c.TimeoutMs = instance.TimeoutMs

	return c, nil
}
