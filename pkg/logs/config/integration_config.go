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

// LogsProcessingRule defines an exclusion or a masking rule to
// be applied on log lines
type LogsProcessingRule struct {
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
	ProcessingRules []LogsProcessingRule `mapstructure:"log_processing_rules" json:"log_processing_rules"`
}

// Validate returns an error if the config is misconfigured
func Validate(config *LogsConfig) (bool, error) {
	switch {
	case config.Type == FileType && config.Path == "":
		return false, fmt.Errorf("file source must have a path")
	case config.Type == TCPType && config.Port == 0:
		return false, fmt.Errorf("tcp source must have a port")
	case config.Type == UDPType && config.Port == 0:
		return false, fmt.Errorf("udp source must have a port")
	}
	return validateProcessingRules(config.ProcessingRules)
}

// validateProcessingRules checks the rules and raises errors if one is misconfigured
func validateProcessingRules(rules []LogsProcessingRule) (bool, error) {
	for _, rule := range rules {
		if rule.Name == "" {
			return false, fmt.Errorf("all processing rules must have a name")
		}
		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch, MaskSequences, MultiLine:
			continue
		case "":
			return false, fmt.Errorf("type must be set for processing rule `%s`", rule.Name)
		default:
			return false, fmt.Errorf("type %s is unsupported for processing rule `%s`", rule.Type, rule.Name)
		}
	}
	return true, nil
}

// Compile compiles all processing rules regular expression.
func Compile(config *LogsConfig) error {
	rules := config.ProcessingRules
	for i, rule := range rules {
		if rule.Pattern == "" {
			return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
		}
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
		default:
			return fmt.Errorf("invalid type for rule %s: %s", rule.Name, rule.Type)
		}
	}
	return nil
}
