// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// Logs source types
const (
	TCPType           = "tcp"
	UDPType           = "udp"
	FileType          = "file"
	DockerType        = "docker"
	JournaldType      = "journald"
	WindowsEventType  = "windows_event"
	StringChannelType = "string_channel"

	// UTF16BE for UTF-16 Big endian encoding
	UTF16BE string = "utf-16-be"
	// UTF16LE for UTF-16 Little Endian encoding
	UTF16LE string = "utf-16-le"
	// SHIFTJIS for Shift JIS (Japanese) encoding
	SHIFTJIS string = "shift-jis"
)

// LogsConfig represents a log source config, which can be for instance
// a file to tail or a port to listen to.
type LogsConfig struct {
	Type string

	Port        int    // Network
	IdleTimeout string `mapstructure:"idle_timeout" json:"idle_timeout"` // Network
	Path        string // File, Journald

	Encoding     string   `mapstructure:"encoding" json:"encoding"`             // File
	ExcludePaths []string `mapstructure:"exclude_paths" json:"exclude_paths"`   // File
	TailingMode  string   `mapstructure:"start_position" json:"start_position"` // File

	IncludeSystemUnits []string `mapstructure:"include_units" json:"include_units"`           // Journald
	ExcludeSystemUnits []string `mapstructure:"exclude_units" json:"exclude_units"`           // Journald
	IncludeUserUnits   []string `mapstructure:"include_user_units" json:"include_user_units"` // Journald
	ExcludeUserUnits   []string `mapstructure:"exclude_user_units" json:"exclude_user_units"` // Journald
	ContainerMode      bool     `mapstructure:"container_mode" json:"container_mode"`         // Journald

	Image string // Docker
	Label string // Docker
	// Name contains the container name
	Name string // Docker
	// Identifier contains the container ID.  This is also set for File sources and used to
	// determine the appropriate tags for the logs.
	Identifier string // Docker, File

	ChannelPath string `mapstructure:"channel_path" json:"channel_path"` // Windows Event
	Query       string // Windows Event

	// used as input only by the Channel tailer.
	// could have been unidirectional but the tailer could not close it in this case.
	Channel chan *ChannelMessage

	// ChannelTags are the tags attached to messages on Channel; unlike Tags this can be
	// modified at runtime (as long as ChannelTagsMutex is held).
	ChannelTags []string

	// ChannelTagsMutex guards ChannelTags.
	ChannelTagsMutex sync.Mutex

	Service         string
	Source          string
	SourceCategory  string
	Tags            []string
	ProcessingRules []*ProcessingRule `mapstructure:"log_processing_rules" json:"log_processing_rules"`

	AutoMultiLine               *bool   `mapstructure:"auto_multi_line_detection" json:"auto_multi_line_detection"`
	AutoMultiLineSampleSize     int     `mapstructure:"auto_multi_line_sample_size" json:"auto_multi_line_sample_size"`
	AutoMultiLineMatchThreshold float64 `mapstructure:"auto_multi_line_match_threshold" json:"auto_multi_line_match_threshold"`
}

// Dump dumps the contents of this struct to a string, for debugging purposes.
func (c *LogsConfig) Dump() string {
	var b strings.Builder
	if c == nil {
		return "&LogsConfig(nil)"
	}
	fmt.Fprintf(&b, "&LogsConfig{\n")
	fmt.Fprintf(&b, "\tType: %#v,\n", c.Type)
	switch c.Type {
	case TCPType:
		fmt.Fprintf(&b, "\tPort: %d,\n", c.Port)
		fmt.Fprintf(&b, "\tIdleTimeout: %#v,\n", c.IdleTimeout)
	case UDPType:
		fmt.Fprintf(&b, "\tPort: %d,\n", c.Port)
		fmt.Fprintf(&b, "\tIdleTimeout: %#v,\n", c.IdleTimeout)
	case FileType:
		fmt.Fprintf(&b, "\tPath: %#v,\n", c.Path)
		fmt.Fprintf(&b, "\tEncoding: %#v,\n", c.Encoding)
		fmt.Fprintf(&b, "\tIdentifier: %#v,\n", c.Identifier)
		fmt.Fprintf(&b, "\tExcludePaths: %#v,\n", c.ExcludePaths)
		fmt.Fprintf(&b, "\tTailingMode: %#v,\n", c.TailingMode)
	case DockerType:
		fmt.Fprintf(&b, "\tImage: %#v,\n", c.Image)
		fmt.Fprintf(&b, "\tLabel: %#v,\n", c.Label)
		fmt.Fprintf(&b, "\tName: %#v,\n", c.Name)
		fmt.Fprintf(&b, "\tIdentifier: %#v,\n", c.Identifier)
	case JournaldType:
		fmt.Fprintf(&b, "\tPath: %#v,\n", c.Path)
		fmt.Fprintf(&b, "\tIncludeSystemUnits: %#v,\n", c.IncludeSystemUnits)
		fmt.Fprintf(&b, "\tExcludeSystemUnits: %#v,\n", c.ExcludeSystemUnits)
		fmt.Fprintf(&b, "\tIncludeUserUnits: %#v,\n", c.IncludeUserUnits)
		fmt.Fprintf(&b, "\tExcludeUserUnits: %#v,\n", c.ExcludeUserUnits)
		fmt.Fprintf(&b, "\tContainerMode: %t,\n", c.ContainerMode)
	case WindowsEventType:
		fmt.Fprintf(&b, "\tChannelPath: %#v,\n", c.ChannelPath)
		fmt.Fprintf(&b, "\tQuery: %#v,\n", c.Query)
	case StringChannelType:
		fmt.Fprintf(&b, "\tChannel: %p,\n", c.Channel)
		c.ChannelTagsMutex.Lock()
		fmt.Fprintf(&b, "\tChannelTags: %#v,\n", c.ChannelTags)
		c.ChannelTagsMutex.Unlock()
	}
	fmt.Fprintf(&b, "\tService: %#v,\n", c.Service)
	fmt.Fprintf(&b, "\tSource: %#v,\n", c.Source)
	fmt.Fprintf(&b, "\tSourceCategory: %#v,\n", c.SourceCategory)
	fmt.Fprintf(&b, "\tTags: %#v,\n", c.Tags)
	fmt.Fprintf(&b, "\tProcessingRules: %#v,\n", c.ProcessingRules)
	if c.AutoMultiLine != nil {
		fmt.Fprintf(&b, "\tAutoMultiLine: %t,\n", *c.AutoMultiLine)
	} else {
		fmt.Fprintf(&b, "\tAutoMultiLine: nil,\n")
	}
	fmt.Fprintf(&b, "\tAutoMultiLineSampleSize: %d,\n", c.AutoMultiLineSampleSize)
	fmt.Fprintf(&b, "\tAutoMultiLineMatchThreshold: %f,\n", c.AutoMultiLineMatchThreshold)
	fmt.Fprintf(&b, "}")
	return b.String()
}

// TailingMode type
type TailingMode uint8

// Tailing Modes
const (
	ForceBeginning = iota
	ForceEnd
	Beginning
	End
)

var tailingModeTuples = []struct {
	s string
	m TailingMode
}{
	{"forceBeginning", ForceBeginning},
	{"forceEnd", ForceEnd},
	{"beginning", Beginning},
	{"end", End},
}

// TailingModeFromString parses a string and returns a corresponding tailing mode, default to End if not found
func TailingModeFromString(mode string) (TailingMode, bool) {
	for _, t := range tailingModeTuples {
		if t.s == mode {
			return t.m, true
		}
	}
	return End, false
}

// TailingModeToString returns seelog string representation for a specified tailing mode. Returns "" for invalid tailing mode.
func (mode TailingMode) String() string {
	for _, t := range tailingModeTuples {
		if t.m == mode {
			return t.s
		}
	}
	return ""
}

// Validate returns an error if the config is misconfigured
func (c *LogsConfig) Validate() error {
	switch {
	case c.Type == "":
		// user don't have to specify a logs-config type when defining
		// an autodiscovery label because so we must override it at some point,
		// this check is mostly used for sanity purposed to detect an override miss.
		return fmt.Errorf("a config must have a type")
	case c.Type == FileType:
		if c.Path == "" {
			return fmt.Errorf("file source must have a path")
		}
		err := c.validateTailingMode()
		if err != nil {
			return err
		}
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

func (c *LogsConfig) validateTailingMode() error {
	mode, found := TailingModeFromString(c.TailingMode)
	if !found && c.TailingMode != "" {
		return fmt.Errorf("invalid tailing mode '%v' for %v", c.TailingMode, c.Path)
	}
	if ContainsWildcard(c.Path) && (mode == Beginning || mode == ForceBeginning) {
		return fmt.Errorf("tailing from the beginning is not supported for wildcard path %v", c.Path)
	}
	return nil
}

// AutoMultiLineEnabled determines whether auto multi line detection is enabled for this config,
// considering both the agent-wide logs_config.auto_multi_line_detection and any config for this
// particular log source.
func (c *LogsConfig) AutoMultiLineEnabled() bool {
	if c.AutoMultiLine != nil {
		return *c.AutoMultiLine
	}
	return config.Datadog.GetBool("logs_config.auto_multi_line_detection")
}

// ContainsWildcard returns true if the path contains any wildcard character
func ContainsWildcard(path string) bool {
	return strings.ContainsAny(path, "*?[")
}
