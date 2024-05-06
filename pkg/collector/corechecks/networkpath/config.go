// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package networkpath

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"gopkg.in/yaml.v2"
	"time"
)

const defaultCheckInterval time.Duration = 1 * time.Minute

// InitConfig is used to deserialize integration init config
type InitConfig struct {
	MinCollectionInterval int `yaml:"min_collection_interval"`
}

// InstanceConfig is used to deserialize integration instance config
type InstanceConfig struct {
	DestHostname string `yaml:"hostname"`

	DestPort uint16 `yaml:"port"`

	MaxTTL uint8 `yaml:"max_ttl"`

	TimeoutMs uint `yaml:"timeout"` // millisecond

	MinCollectionInterval int `yaml:"min_collection_interval"`

	Tags []string `yaml:"tags"`
}

// CheckConfig defines the configuration of the
// Network Path integration
type CheckConfig struct {
	DestHostname          string
	DestPort              uint16
	MaxTTL                uint8
	TimeoutMs             uint
	MinCollectionInterval time.Duration
	Tags                  []string
}

// NewCheckConfig builds a new check config
func NewCheckConfig(rawInstance integration.Data, rawInitConfig integration.Data) (*CheckConfig, error) {
	instance := InstanceConfig{}
	initConfig := InitConfig{}

	err := yaml.Unmarshal(rawInitConfig, &initConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid init_config: %s", err)
	}

	err = yaml.Unmarshal(rawInstance, &instance)
	if err != nil {
		return nil, fmt.Errorf("invalid instance config: %s", err)
	}

	c := &CheckConfig{}

	c.DestHostname = instance.DestHostname
	c.DestPort = instance.DestPort
	c.MaxTTL = instance.MaxTTL
	c.TimeoutMs = instance.TimeoutMs

	c.MinCollectionInterval = firstNonZero(
		time.Duration(instance.MinCollectionInterval)*time.Second,
		time.Duration(initConfig.MinCollectionInterval)*time.Second,
		defaultCheckInterval,
	)
	if c.MinCollectionInterval <= 0 {
		return nil, fmt.Errorf("min collection interval must be > 0")
	}

	c.Tags = instance.Tags

	return c, nil
}

func firstNonZero(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
