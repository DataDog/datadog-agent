// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"fmt"
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

// LogsConfig represents a log source config, which can be for instance
// a file to tail or a port to listen to.
type LogsConfig struct {
	Type string

	Port int    // Network
	Path string // File, Journald

	IncludeUnits []string `mapstructure:"include_units" json:"include_units"` // Journald
	ExcludeUnits []string `mapstructure:"exclude_units" json:"exclude_units"` // Journald

	Image      string // Docker
	Label      string // Docker
	Name       string // Docker
	Identifier string // Docker

	ChannelPath string `mapstructure:"channel_path" json:"channel_path"` // Windows Event
	Query       string // Windows Event

	Service         string
	Source          string
	SourceCategory  string
	Tags            []string
	ProcessingRules []*ProcessingRule `mapstructure:"log_processing_rules" json:"log_processing_rules"`
}

// Validate returns an error if the config is misconfigured
func (c *LogsConfig) Validate() error {
	switch {
	case c.Type == "":
		// user don't have to specify a logs-config type when defining
		// an autodiscovery label because so we must override it at some point,
		// this check is mostly used for sanity purposed to detect an override miss.
		return fmt.Errorf("a config must have a type")
	case c.Type == FileType && c.Path == "":
		return fmt.Errorf("file source must have a path")
	case c.Type == TCPType && c.Port == 0:
		return fmt.Errorf("tcp source must have a port")
	case c.Type == UDPType && c.Port == 0:
		return fmt.Errorf("udp source must have a port")
	}
	err := ValidateProcessingRules(c.ProcessingRules)
	if err != nil {
		return err
	}
	return CompileProcessingRules(c.ProcessingRules)
}
