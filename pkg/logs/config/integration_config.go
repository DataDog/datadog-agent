// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"regexp"
)

// Logs source types
const (
	TCPType          = "tcp"
	UDPType          = "udp"
	FileType         = "file"
	DockerType       = "docker"
	JournaldType     = "journald"
	WindowsEventType = "windows_event"
)

// Logs rule types
const (
	ExcludeAtMatch = "exclude_at_match"
	IncludeAtMatch = "include_at_match"
	MaskSequences  = "mask_sequences"
	MultiLine      = "multi_line"
)

// ProcessingRule defines an exclusion or a masking rule to
// be applied on log lines
type ProcessingRule struct {
	Type               string
	Name               string
	ReplacePlaceholder string `mapstructure:"replace_placeholder" json:"replace_placeholder"`
	Pattern            string
	// TODO: should be moved out
	Reg                     *regexp.Regexp
	ReplacePlaceholderBytes []byte
}

// LogsConfig represents a log source config, which can be for instance
// a file to tail or a port to listen to.
type LogsConfig struct {
	Type string

	Port int    // Network
	Path string // File, Journald

	IncludeUnits []string `mapstructure:"include_units" json:"include_units"` // Journald
	ExcludeUnits []string `mapstructure:"exclude_units" json:"exclude_units"` // Journald

	Image string // Docker
	Label string // Docker
	Name  string // Docker

	ChannelPath string `mapstructure:"channel_path" json:"channel_path"` // Windows Event
	Query       string // Windows Event

	Service         string
	Source          string
	SourceCategory  string
	Tags            []string
	ProcessingRules []ProcessingRule `mapstructure:"log_processing_rules" json:"log_processing_rules"`
}

// Validate returns an error if the config is misconfigured
func (c *LogsConfig) Validate() error {
	switch {
	case c.Type == FileType && c.Path == "":
		return fmt.Errorf("file source must have a path")
	case c.Type == TCPType && c.Port == 0:
		return fmt.Errorf("tcp source must have a port")
	case c.Type == UDPType && c.Port == 0:
		return fmt.Errorf("udp source must have a port")
	}
	return c.validateProcessingRules()
}

// validateProcessingRules validates the rules and raises an error if one is misconfigured.
// All sources must have:
// - a valid name
// - a valid type
// - a valid pattern that compiles
func (c *LogsConfig) validateProcessingRules() error {
	for _, rule := range c.ProcessingRules {
		if rule.Name == "" {
			return fmt.Errorf("all processing rules must have a name")
		}

		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch, MaskSequences, MultiLine:
			break
		case "":
			return fmt.Errorf("type must be set for processing rule `%s`", rule.Name)
		default:
			return fmt.Errorf("type %s is not supported for processing rule `%s`", rule.Type, rule.Name)
		}

		if rule.Pattern == "" {
			return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
		}
		_, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return fmt.Errorf("invalid pattern %s for processing rule: %s", rule.Pattern, rule.Name)
		}
	}
	return nil
}

// Compile compiles all processing rule regular expressions.
func (c *LogsConfig) Compile() error {
	rules := c.ProcessingRules
	for i, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return err
		}
		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch:
			rules[i].Reg = re
		case MaskSequences:
			rules[i].Reg = re
			rules[i].ReplacePlaceholderBytes = []byte(rule.ReplacePlaceholder)
		case MultiLine:
			rules[i].Reg, err = regexp.Compile("^" + rule.Pattern)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
