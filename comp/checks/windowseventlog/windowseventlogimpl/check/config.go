// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	yaml "go.yaml.in/yaml/v2"
)

const (
	defaultConfigQuery             = "*"
	defaultConfigStart             = "now"
	defaultConfigPayloadSize       = 10
	defaultConfigTagEventID        = false
	defaultConfigTagSID            = false
	defaultConfigEventPriority     = "normal"
	defaultConfigAuthType          = "default"
	defaultConfigInterpretMessages = true
	// Legacy mode options have special handling, see processLegacyModeOptions()
	defaultConfigLegacyMode   = false
	defaultConfigLegacyModeV2 = false
)

// Config represents the Windows Event Log check configuration and its yaml marshalling
type Config struct {
	instance instanceConfig
	init     initConfig
}

type instanceConfig struct {
	DDSecurityEvents  option.Option[string]        `yaml:"dd_security_events"`
	ChannelPath       option.Option[string]        `yaml:"path"`
	Query             option.Option[string]        `yaml:"query"`
	Start             option.Option[string]        `yaml:"start"`
	Timeout           option.Option[int]           `yaml:"timeout"`
	PayloadSize       option.Option[int]           `yaml:"payload_size"`
	BookmarkFrequency option.Option[int]           `yaml:"bookmark_frequency"`
	LegacyMode        option.Option[bool]          `yaml:"legacy_mode"`
	LegacyModeV2      option.Option[bool]          `yaml:"legacy_mode_v2"`
	EventPriority     option.Option[string]        `yaml:"event_priority"`
	TagEventID        option.Option[bool]          `yaml:"tag_event_id"`
	TagSID            option.Option[bool]          `yaml:"tag_sid"`
	Filters           option.Option[filtersConfig] `yaml:"filters"`
	IncludedMessages  option.Option[[]string]      `yaml:"included_messages"`
	ExcludedMessages  option.Option[[]string]      `yaml:"excluded_messages"`
	AuthType          option.Option[string]        `yaml:"auth_type"`
	Server            option.Option[string]        `yaml:"server"`
	User              option.Option[string]        `yaml:"user"`
	Domain            option.Option[string]        `yaml:"domain"`
	Password          option.Option[string]        `yaml:"password"`
	InterpretMessages option.Option[bool]          `yaml:"interpret_messages"`
}

type filtersConfig struct {
	SourceList []string `yaml:"source"`
	TypeList   []string `yaml:"type"`
	IDList     []int    `yaml:"id"`
}

type initConfig struct {
	TagEventID        option.Option[bool]   `yaml:"tag_event_id"`
	TagSID            option.Option[bool]   `yaml:"tag_sid"`
	EventPriority     option.Option[string] `yaml:"event_priority"`
	InterpretMessages option.Option[bool]   `yaml:"interpret_messages"`
	LegacyMode        option.Option[bool]   `yaml:"legacy_mode"`
	LegacyModeV2      option.Option[bool]   `yaml:"legacy_mode_v2"`
}

func (f *filtersConfig) Sources() []string {
	return f.SourceList
}
func (f *filtersConfig) Types() []string {
	return f.TypeList
}
func (f *filtersConfig) IDs() []int {
	return f.IDList
}

func unmarshalConfig(instance integration.Data, initConfig integration.Data) (*Config, error) {
	var c Config

	err := c.unmarshal(instance, initConfig)
	if err != nil {
		return nil, fmt.Errorf("yaml parsing error: %w", err)
	}

	err = c.genQuery()
	if err != nil {
		return nil, fmt.Errorf("error generating query from filters: %w", err)
	}

	c.setDefaults()

	return &c, nil
}

func (c *Config) unmarshal(instance integration.Data, initConfig integration.Data) error {
	// Unmarshal config
	err := yaml.Unmarshal(instance, &c.instance)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(initConfig, &c.init)
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) genQuery() error {
	if _, isSet := c.instance.Query.Get(); isSet {
		return nil
	}
	filters, isSet := c.instance.Filters.Get()
	if !isSet {
		c.instance.Query.Set(defaultConfigQuery)
		return nil
	}
	query, err := queryFromFilter(&filters)
	if err != nil {
		return err
	}
	c.instance.Query.Set(query)
	return nil
}

func setOptionalDefault[T any](optional *option.Option[T], def T) {
	optional.SetIfNone(def)
}

func setOptionalDefaultWithInitConfig[T any](instance *option.Option[T], shared option.Option[T], def T) {
	instance.SetOptionIfNone(shared)
	instance.SetIfNone(def)
}

// Sets default values for the instance configuration.
// initConfig fields will override hardcoded defaults.
func (c *Config) setDefaults() {
	//
	// instance fields
	//
	setOptionalDefault(&c.instance.Query, defaultConfigQuery)
	setOptionalDefault(&c.instance.Start, defaultConfigStart)
	setOptionalDefault(&c.instance.PayloadSize, defaultConfigPayloadSize)
	// bookmark frequency defaults to the payload size
	defaultBookmarkFrequency, _ := c.instance.PayloadSize.Get()
	setOptionalDefault(&c.instance.BookmarkFrequency, defaultBookmarkFrequency)
	setOptionalDefault(&c.instance.AuthType, defaultConfigAuthType)

	//
	// instance fields with initConfig defaults
	//
	setOptionalDefaultWithInitConfig(&c.instance.TagEventID, c.init.TagEventID, defaultConfigTagEventID)
	setOptionalDefaultWithInitConfig(&c.instance.TagSID, c.init.TagSID, defaultConfigTagSID)
	setOptionalDefaultWithInitConfig(&c.instance.EventPriority, c.init.EventPriority, defaultConfigEventPriority)
	setOptionalDefaultWithInitConfig(&c.instance.InterpretMessages, c.init.InterpretMessages, defaultConfigInterpretMessages)

	// Legacy mode options
	c.processLegacyModeOptions()
}

func (c *Config) processLegacyModeOptions() {
	// use initConfig option if instance value is unset
	c.instance.LegacyMode.SetOptionIfNone(c.init.LegacyMode)
	c.instance.LegacyModeV2.SetOptionIfNone(c.init.LegacyModeV2)

	// If legacy_mode and legacy_mode_v2 are unset, default to legacy mode for configuration backwards compatibility
	if _, isSet := c.instance.LegacyMode.Get(); !isSet && !isaffirmative(c.instance.LegacyModeV2) {
		c.instance.LegacyMode.Set(true)
	}

	// if option is unset, default to false
	setOptionalDefault(&c.instance.LegacyMode, defaultConfigLegacyMode)
	setOptionalDefault(&c.instance.LegacyModeV2, defaultConfigLegacyModeV2)
}
