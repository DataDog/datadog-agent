// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package config parses and validates gNMI check instance and profile configuration.
package config

import (
	"errors"
	"fmt"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
	gnmi "github.com/openconfig/gnmi/proto/gnmi"
)

const (
	// DefaultPort is the default gNMI gRPC port used when none is configured.
	DefaultPort = 57400
	// DefaultMinCollectionInterval is the default collection interval in seconds.
	DefaultMinCollectionInterval = int(defaults.DefaultCheckInterval / time.Second)
)

// InstanceConfig holds the YAML instance configuration for a gNMI check.
type InstanceConfig struct {
	Address                    string   `yaml:"address"`
	Port                       int      `yaml:"port"`
	Username                   string   `yaml:"username"`
	Password                   string   `yaml:"password"`
	Profile                    string   `yaml:"profile"`
	MinCollectionInterval      int      `yaml:"min_collection_interval"`
	MetadataCollectionInterval int      `yaml:"metadata_collection_interval"`
	Tags                       []string `yaml:"tags"`
	CollectTopology            bool     `yaml:"collect_topology"`
	UseTLS                     bool     `yaml:"use_tls"`
	InsecureSkipVerify         bool     `yaml:"insecure_skip_verify"`
	Encoding                   string   `yaml:"encoding"`
}

// CheckConfig combines a validated instance config with its loaded profile.
type CheckConfig struct {
	Instance InstanceConfig
	Profile  ProfileDefinition
}

// NewCheckConfig parses instance YAML, validates it, and loads the referenced profile.
func NewCheckConfig(rawInstance integration.Data) (*CheckConfig, error) {
	instance, err := ParseInstanceConfig(rawInstance)
	if err != nil {
		return nil, err
	}

	profile, err := LoadProfile(instance.Profile)
	if err != nil {
		return nil, err
	}

	return &CheckConfig{
		Instance: *instance,
		Profile:  *profile,
	}, nil
}

// ParseInstanceConfig parses and validates instance YAML without loading the profile.
func ParseInstanceConfig(rawInstance integration.Data) (*InstanceConfig, error) {
	instance := InstanceConfig{
		Port:                  DefaultPort,
		MinCollectionInterval: DefaultMinCollectionInterval,
	}

	if err := yaml.Unmarshal(rawInstance, &instance); err != nil {
		return nil, fmt.Errorf("parse instance config: %w", err)
	}

	if err := validateInstanceConfig(&instance); err != nil {
		return nil, err
	}

	return &instance, nil
}

func validateInstanceConfig(instance *InstanceConfig) error {
	if instance.Address == "" {
		return errors.New("`address` is required")
	}
	if instance.Username == "" {
		return errors.New("`username` is required")
	}
	if instance.Password == "" {
		return errors.New("`password` is required")
	}
	if instance.Profile == "" {
		return errors.New("`profile` is required")
	}
	if instance.Port <= 0 || instance.Port > 65535 {
		return fmt.Errorf("invalid `port` %d: must be between 1 and 65535", instance.Port)
	}
	if instance.MinCollectionInterval <= 0 {
		return fmt.Errorf("invalid `min_collection_interval` %d: must be greater than 0", instance.MinCollectionInterval)
	}
	if instance.MetadataCollectionInterval < 0 {
		return fmt.Errorf("invalid `metadata_collection_interval` %d: must be greater than or equal to 0", instance.MetadataCollectionInterval)
	}
	if _, err := ParseEncoding(instance.Encoding); err != nil {
		return err
	}
	return nil
}

// ResolvedEncoding returns the configured gNMI subscribe encoding.
func (c *InstanceConfig) ResolvedEncoding() (gnmi.Encoding, error) {
	return ParseEncoding(c.Encoding)
}

// String returns a redacted representation safe for logs and error messages.
func (c *CheckConfig) String() string {
	return fmt.Sprintf("CheckConfig{Address=`%s`, Port=`%d`, Username=`%s`, Profile=`%s`, MinCollectionInterval=`%d`, CollectTopology=`%t`, MetricCount=`%d`}",
		c.Instance.Address,
		c.Instance.Port,
		c.Instance.Username,
		c.Instance.Profile,
		c.Instance.MinCollectionInterval,
		c.Instance.CollectTopology,
		len(c.Profile.Metrics),
	)
}

// String returns a redacted representation safe for logs and error messages.
func (c *InstanceConfig) String() string {
	return fmt.Sprintf("InstanceConfig{Address=`%s`, Port=`%d`, Username=`%s`, Profile=`%s`, MinCollectionInterval=`%d`, CollectTopology=`%t`}",
		c.Address,
		c.Port,
		c.Username,
		c.Profile,
		c.MinCollectionInterval,
		c.CollectTopology,
	)
}
