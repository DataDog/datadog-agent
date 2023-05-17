// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtlog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	yaml "gopkg.in/yaml.v2"
)

const (
	defaultConfigQuery          = "*"
	defaultConfigStart          = "now"
	defaultConfigTimeout        = 5
	defaultConfigPayload_size   = 10
	defaultConfigTag_event_id   = false
	defaultConfigTag_sid        = false
	defaultConfigEvent_priority = "normal"
)

type Config struct {
	instance instanceConfig
	init     initConfig
}

type instanceConfig struct {
	ChannelPath        *string        `yaml:"path"`
	Query              *string        `yaml:"query"`
	Start              *string        `yaml:"start"`
	Timeout            *int           `yaml:"timeout"`
	Payload_size       *int           `yaml:"payload_size"`
	Bookmark_frequency *int           `yaml:"bookmark_frequency"`
	Legacy_mode        *bool          `yaml:"legacy_mode"`
	Event_priority     *string        `yaml:"event_priority"`
	Tag_event_id       *bool          `yaml:"tag_event_id"`
	Tag_sid            *bool          `yaml:"tag_sid"`
	Filters            *filtersConfig `yaml:"filters"`
}

type filtersConfig struct {
	SourceList []string `yaml:"source"`
	TypeList   []string `yaml:"type"`
	IDList     []int    `yaml:"id"`
}

type initConfig struct {
	Tag_event_id   *bool   `yaml:"tag_event_id"`
	Tag_sid        *bool   `yaml:"tag_sid"`
	Event_priority *string `yaml:"event_priority"`
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

func UnmarshalConfig(instance integration.Data, initConfig integration.Data) (*Config, error) {
	var c Config

	err := c.unmarshal(instance, initConfig)
	if err != nil {
		return nil, fmt.Errorf("yaml parsing error: %v", err)
	}

	c.genQuery()

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

	if c.instance.Payload_size == nil {
		def := defaultConfigPayload_size
		c.instance.Payload_size = &def
	}

	if c.instance.Bookmark_frequency == nil {
		def := *c.instance.Payload_size
		c.instance.Bookmark_frequency = &def
	}

	if c.instance.Legacy_mode == nil {
		def := false
		c.instance.Legacy_mode = &def
	}

	// instance fields with initConfig defaults
	if c.instance.Tag_event_id == nil {
		def := defaultConfigTag_event_id
		if c.init.Tag_event_id != nil {
			def = *c.init.Tag_event_id
		}
		c.instance.Tag_event_id = &def
	}

	if c.instance.Tag_sid == nil {
		def := defaultConfigTag_sid
		if c.init.Tag_sid != nil {
			def = *c.init.Tag_sid
		}
		c.instance.Tag_sid = &def
	}

	if c.instance.Event_priority == nil {
		def := defaultConfigEvent_priority
		if c.init.Event_priority != nil {
			def = *c.init.Event_priority
		}
		c.instance.Event_priority = &def
	}
}
