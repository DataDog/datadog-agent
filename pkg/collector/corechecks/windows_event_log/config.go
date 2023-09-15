// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows

package evtlog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	yaml "gopkg.in/yaml.v2"
)

const (
	defaultConfigQuery         = "*"
	defaultConfigStart         = "now"
	defaultConfigTimeout       = 5
	defaultConfigPayloadSize   = 10
	defaultConfigTagEventID    = false
	defaultConfigTagSID        = false
	defaultConfigEventPriority = "normal"
)

// Config represents the Windows Event Log check configuration and its yaml marshalling
type Config struct {
	instance instanceConfig
	init     initConfig
}

type instanceConfig struct {
	ChannelPath       *string        `yaml:"path"`
	Query             *string        `yaml:"query"`
	Start             *string        `yaml:"start"`
	Timeout           *int           `yaml:"timeout"`
	PayloadSize       *int           `yaml:"payload_size"`
	BookmarkFrequency *int           `yaml:"bookmark_frequency"`
	LegacyMode        *bool          `yaml:"legacy_mode"`
	EventPriority     *string        `yaml:"event_priority"`
	TagEventID        *bool          `yaml:"tag_event_id"`
	TagSID            *bool          `yaml:"tag_sid"`
	Filters           *filtersConfig `yaml:"filters"`
	IncludedMessages  []string       `yaml:"included_messages"`
	ExcludedMessages  []string       `yaml:"excluded_messages"`
}

type filtersConfig struct {
	SourceList []string `yaml:"source"`
	TypeList   []string `yaml:"type"`
	IDList     []int    `yaml:"id"`
}

type initConfig struct {
	TagEventID    *bool   `yaml:"tag_event_id"`
	TagSID        *bool   `yaml:"tag_sid"`
	EventPriority *string `yaml:"event_priority"`
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
		return nil, fmt.Errorf("yaml parsing error: %v", err)
	}

	err = c.genQuery()
	if err != nil {
		return nil, fmt.Errorf("error generating query from filters: %v", err)
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
	if c.instance.Query != nil {
		return nil
	}
	if c.instance.Filters == nil {
		def := defaultConfigQuery
		c.instance.Query = &def
		return nil
	}
	query, err := queryFromFilter(c.instance.Filters)
	if err != nil {
		return err
	}
	c.instance.Query = &query
	return nil
}

// Sets default values for the instance configuration.
// initConfig fields will override hardcoded defaults.
func (c *Config) setDefaults() {
	// instance fields
	if c.instance.ChannelPath == nil {
		def := ""
		c.instance.ChannelPath = &def
	}

	if c.instance.Query == nil {
		def := defaultConfigQuery
		c.instance.Query = &def
	}

	if c.instance.Start == nil {
		def := defaultConfigStart
		c.instance.Start = &def
	}

	if c.instance.Timeout == nil {
		def := defaultConfigTimeout
		c.instance.Timeout = &def
	}

	if c.instance.PayloadSize == nil {
		def := defaultConfigPayloadSize
		c.instance.PayloadSize = &def
	}

	if c.instance.BookmarkFrequency == nil {
		def := *c.instance.PayloadSize
		c.instance.BookmarkFrequency = &def
	}

	if c.instance.LegacyMode == nil {
		def := false
		c.instance.LegacyMode = &def
	}

	// instance fields with initConfig defaults
	if c.instance.TagEventID == nil {
		def := defaultConfigTagEventID
		if c.init.TagEventID != nil {
			def = *c.init.TagEventID
		}
		c.instance.TagEventID = &def
	}

	if c.instance.TagSID == nil {
		def := defaultConfigTagSID
		if c.init.TagSID != nil {
			def = *c.init.TagSID
		}
		c.instance.TagSID = &def
	}

	if c.instance.EventPriority == nil {
		def := defaultConfigEventPriority
		if c.init.EventPriority != nil {
			def = *c.init.EventPriority
		}
		c.instance.EventPriority = &def
	}
}
