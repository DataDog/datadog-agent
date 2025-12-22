// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package networkpath

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

const (
	defaultCheckInterval time.Duration = 1 * time.Minute
)

// Number is a type that is used to make a generic version
// of the firstNonZero function
type Number interface {
	~int | ~int64 | ~uint8
}

// InitConfig is used to deserialize integration init config
type InitConfig struct {
	MinCollectionInterval int64 `yaml:"min_collection_interval"`
	TimeoutMs             int64 `yaml:"timeout"`
	MaxTTL                uint8 `yaml:"max_ttl"`
	TracerouteQueries     int   `yaml:"traceroute_queries"`
	E2eQueries            int   `yaml:"e2e_queries"`
}

// InstanceConfig is used to deserialize integration instance config
type InstanceConfig struct {
	DestHostname string `yaml:"hostname"`

	DestPort uint16 `yaml:"port"`

	Protocol  string `yaml:"protocol"`
	TCPMethod string `yaml:"tcp_method"`
	// TCPSynParisTracerouteMode makes TCP SYN traceroute act like paris traceroute (fixed packet ID, randomized seq)
	TCPSynParisTracerouteMode bool `yaml:"tcp_syn_paris_traceroute_mode"`
	// DisableWindowsDriver disables the use of Windows driver for traceroute
	DisableWindowsDriver bool `yaml:"disable_windows_driver"`

	SourceService      string `yaml:"source_service"`
	DestinationService string `yaml:"destination_service"`

	MaxTTL uint8 `yaml:"max_ttl"`

	TimeoutMs int64 `yaml:"timeout"`

	MinCollectionInterval int `yaml:"min_collection_interval"`

	TracerouteQueries int `yaml:"traceroute_queries"`
	E2eQueries        int `yaml:"e2e_queries"`

	Tags []string `yaml:"tags"`
}

// CheckConfig defines the configuration of the
// Network Path integration
type CheckConfig struct {
	DestHostname       string
	DestPort           uint16
	SourceService      string
	DestinationService string
	MaxTTL             uint8
	Protocol           payload.Protocol
	TCPMethod          payload.TCPMethod
	// TCPSynParisTracerouteMode makes TCP SYN traceroute act like paris traceroute (fixed packet ID, randomized seq)
	TCPSynParisTracerouteMode bool
	// DisableWindowsDriver disables the use of Windows driver for traceroute
	DisableWindowsDriver  bool
	Timeout               time.Duration
	MinCollectionInterval time.Duration
	TracerouteQueries     int
	E2eQueries            int
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

	// hostname validation is done by the datadog-traceroute library but an empty hostname results in querying system-probe with an invalid URL
	if instance.DestHostname == "" {
		return nil, errors.New("invalid instance config, hostname must be provided")
	}

	c := &CheckConfig{}

	c.DestHostname = instance.DestHostname
	c.DestPort = instance.DestPort
	c.SourceService = instance.SourceService
	c.DestinationService = instance.DestinationService
	c.Protocol = payload.Protocol(strings.ToUpper(instance.Protocol))
	c.TCPMethod = payload.MakeTCPMethod(instance.TCPMethod)
	c.TCPSynParisTracerouteMode = instance.TCPSynParisTracerouteMode
	c.DisableWindowsDriver = instance.DisableWindowsDriver

	c.MinCollectionInterval = firstNonZero(
		time.Duration(instance.MinCollectionInterval)*time.Second,
		time.Duration(initConfig.MinCollectionInterval)*time.Second,
		defaultCheckInterval,
	)
	if c.MinCollectionInterval <= 0 {
		return nil, errors.New("min collection interval must be > 0")
	}

	c.Timeout = firstNonZero(
		time.Duration(instance.TimeoutMs)*time.Millisecond,
		time.Duration(initConfig.TimeoutMs)*time.Millisecond,
		setup.DefaultNetworkPathTimeout*time.Millisecond,
	)
	if c.Timeout <= 0 {
		return nil, errors.New("timeout must be > 0")
	}

	c.MaxTTL = firstNonZero(
		instance.MaxTTL,
		initConfig.MaxTTL,
		setup.DefaultNetworkPathMaxTTL,
	)

	c.TracerouteQueries = firstNonZero(
		instance.TracerouteQueries,
		initConfig.TracerouteQueries,
		setup.DefaultNetworkPathStaticPathTracerouteQueries,
	)

	c.E2eQueries = firstNonZero(
		instance.E2eQueries,
		initConfig.E2eQueries,
		setup.DefaultNetworkPathStaticPathE2eQueries,
	)

	c.Tags = instance.Tags
	c.Namespace = setup.Datadog().GetString("network_devices.namespace")

	return c, nil
}

func firstNonZero[T Number](values ...T) T {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
