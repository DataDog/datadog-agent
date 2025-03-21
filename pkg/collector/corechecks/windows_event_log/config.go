// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	yaml "gopkg.in/yaml.v2"
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
	ChannelPath       optional.Option[string]        `yaml:"path"`
	Query             optional.Option[string]        `yaml:"query"`
	Start             optional.Option[string]        `yaml:"start"`
	Timeout           optional.Option[int]           `yaml:"timeout"`
	PayloadSize       optional.Option[int]           `yaml:"payload_size"`
	BookmarkFrequency optional.Option[int]           `yaml:"bookmark_frequency"`
	LegacyMode        optional.Option[bool]          `yaml:"legacy_mode"`
	LegacyModeV2      optional.Option[bool]          `yaml:"legacy_mode_v2"`
	EventPriority     optional.Option[string]        `yaml:"event_priority"`
	TagEventID        optional.Option[bool]          `yaml:"tag_event_id"`
	TagSID            optional.Option[bool]          `yaml:"tag_sid"`
	Filters           optional.Option[filtersConfig] `yaml:"filters"`
	IncludedMessages  optional.Option[[]string]      `yaml:"included_messages"`
	ExcludedMessages  optional.Option[[]string]      `yaml:"excluded_messages"`
	AuthType          optional.Option[string]        `yaml:"auth_type"`
	Server            optional.Option[string]        `yaml:"server"`
	User              optional.Option[string]        `yaml:"user"`
	Domain            optional.Option[string]        `yaml:"domain"`
	Password          optional.Option[string]        `yaml:"password"`
	InterpretMessages optional.Option[bool]          `yaml:"interpret_messages"`
}

type filtersConfig struct {
	SourceList []string `yaml:"source"`
	TypeList   []string `yaml:"type"`
	IDList     []int    `yaml:"id"`
}

type initConfig struct {
	TagEventID        optional.Option[bool]   `yaml:"tag_event_id"`
	TagSID            optional.Option[bool]   `yaml:"tag_sid"`
	EventPriority     optional.Option[string] `yaml:"event_priority"`
	InterpretMessages optional.Option[bool]   `yaml:"interpret_messages"`
	LegacyMode        optional.Option[bool]   `yaml:"legacy_mode"`
	LegacyModeV2      optional.Option[bool]   `yaml:"legacy_mode_v2"`
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

func setOptionalDefault[T any](optional *optional.Option[T], def T) {
	optional.SetIfNone(def)
}

func setOptionalDefaultWithInitConfig[T any](instance *optional.Option[T], shared optional.Option[T], def T) {
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
