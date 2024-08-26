// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package networkpath

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"gopkg.in/yaml.v2"
)

const (
	defaultCheckInterval time.Duration = 1 * time.Minute
	defaultTimeout       time.Duration = 10 * time.Second
)

// InitConfig is used to deserialize integration init config
type InitConfig struct {
	MinCollectionInterval int64 `yaml:"min_collection_interval"`
	TimeoutMs             int64 `yaml:"timeout"`
}

// InstanceConfig is used to deserialize integration instance config
type InstanceConfig struct {
	DestHostname string `yaml:"hostname"`

	DestPort uint16 `yaml:"port"`

	Protocol string `yaml:"protocol"`

	SourceService      string `yaml:"source_service"`
	DestinationService string `yaml:"destination_service"`

	MaxTTL uint8 `yaml:"max_ttl"`

	TimeoutMs int64 `yaml:"timeout"`

	MinCollectionInterval int `yaml:"min_collection_interval"`

	Tags []string `yaml:"tags"`
}

// CheckConfig defines the configuration of the
// Network Path integration
type CheckConfig struct {
	DestHostname          string
	DestPort              uint16
	SourceService         string
	DestinationService    string
	MaxTTL                uint8
	Protocol              payload.Protocol
	Timeout               time.Duration
	MinCollectionInterval time.Duration
	Tags                  []string
	Namespace             string
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
	c.SourceService = instance.SourceService
	c.DestinationService = instance.DestinationService
	c.MaxTTL = instance.MaxTTL
	c.Protocol = payload.Protocol(strings.ToUpper(instance.Protocol))

	c.MinCollectionInterval = firstNonZero(
		time.Duration(instance.MinCollectionInterval)*time.Second,
		time.Duration(initConfig.MinCollectionInterval)*time.Second,
		defaultCheckInterval,
	)
	if c.MinCollectionInterval <= 0 {
		return nil, fmt.Errorf("min collection interval must be > 0")
	}

	c.Timeout = firstNonZero(
		time.Duration(instance.TimeoutMs)*time.Millisecond,
		time.Duration(initConfig.TimeoutMs)*time.Millisecond,
		defaultTimeout,
	)
	if c.Timeout <= 0 {
		return nil, fmt.Errorf("timeout must be > 0")
	}

	c.Tags = instance.Tags
	c.Namespace = coreconfig.Datadog().GetString("network_devices.namespace")

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
