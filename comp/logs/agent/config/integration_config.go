// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Logs source types
const (
	TCPType           = "tcp"
	UDPType           = "udp"
	FileType          = "file"
	DockerType        = "docker"
	ContainerdType    = "containerd"
	JournaldType      = "journald"
	IntegrationType   = "integration"
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

	IntegrationName string

	Port        int    // Network
	IdleTimeout string `mapstructure:"idle_timeout" json:"idle_timeout"` // Network
	Path        string // File, Journald

	Encoding     string   `mapstructure:"encoding" json:"encoding"`             // File
	ExcludePaths []string `mapstructure:"exclude_paths" json:"exclude_paths"`   // File
	TailingMode  string   `mapstructure:"start_position" json:"start_position"` // File

	//nolint:revive // TODO(AML) Fix revive linter
	ConfigId           string   `mapstructure:"config_id" json:"config_id"`                   // Journald
	IncludeSystemUnits []string `mapstructure:"include_units" json:"include_units"`           // Journald
	ExcludeSystemUnits []string `mapstructure:"exclude_units" json:"exclude_units"`           // Journald
	IncludeUserUnits   []string `mapstructure:"include_user_units" json:"include_user_units"` // Journald
	ExcludeUserUnits   []string `mapstructure:"exclude_user_units" json:"exclude_user_units"` // Journald
	IncludeMatches     []string `mapstructure:"include_matches" json:"include_matches"`       // Journald
	ExcludeMatches     []string `mapstructure:"exclude_matches" json:"exclude_matches"`       // Journald
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
	// ProcessRawMessage is used to process the raw message instead of only the content part of the message.
	ProcessRawMessage *bool `mapstructure:"process_raw_message" json:"process_raw_message"`

	AutoMultiLine               *bool   `mapstructure:"auto_multi_line_detection" json:"auto_multi_line_detection"`
	AutoMultiLineSampleSize     int     `mapstructure:"auto_multi_line_sample_size" json:"auto_multi_line_sample_size"`
	AutoMultiLineMatchThreshold float64 `mapstructure:"auto_multi_line_match_threshold" json:"auto_multi_line_match_threshold"`
}

// Dump dumps the contents of this struct to a string, for debugging purposes.
func (c *LogsConfig) Dump(multiline bool) string {
	if c == nil {
		return "&LogsConfig(nil)"
	}

	var b strings.Builder
	ws := func(fmt string) string {
		if multiline {
			return "\n\t" + fmt
		}
		return " " + fmt
	}

	fmt.Fprint(&b, ws("&LogsConfig{"))
	fmt.Fprintf(&b, ws("Type: %#v,"), c.Type)
	switch c.Type {
	case TCPType:
		fmt.Fprintf(&b, ws("Port: %d,"), c.Port)
		fmt.Fprintf(&b, ws("IdleTimeout: %#v,"), c.IdleTimeout)
	case UDPType:
		fmt.Fprintf(&b, ws("Port: %d,"), c.Port)
		fmt.Fprintf(&b, ws("IdleTimeout: %#v,"), c.IdleTimeout)
	case FileType:
		fmt.Fprintf(&b, ws("Path: %#v,"), c.Path)
		fmt.Fprintf(&b, ws("Encoding: %#v,"), c.Encoding)
		fmt.Fprintf(&b, ws("Identifier: %#v,"), c.Identifier)
		fmt.Fprintf(&b, ws("ExcludePaths: %#v,"), c.ExcludePaths)
		fmt.Fprintf(&b, ws("TailingMode: %#v,"), c.TailingMode)
	case DockerType, ContainerdType:
		fmt.Fprintf(&b, ws("Image: %#v,"), c.Image)
		fmt.Fprintf(&b, ws("Label: %#v,"), c.Label)
		fmt.Fprintf(&b, ws("Name: %#v,"), c.Name)
		fmt.Fprintf(&b, ws("Identifier: %#v,"), c.Identifier)
	case JournaldType:
		fmt.Fprintf(&b, ws("Path: %#v,"), c.Path)
		fmt.Fprintf(&b, ws("IncludeSystemUnits: %#v,"), c.IncludeSystemUnits)
		fmt.Fprintf(&b, ws("ExcludeSystemUnits: %#v,"), c.ExcludeSystemUnits)
		fmt.Fprintf(&b, ws("IncludeUserUnits: %#v,"), c.IncludeUserUnits)
		fmt.Fprintf(&b, ws("ExcludeUserUnits: %#v,"), c.ExcludeUserUnits)
		fmt.Fprintf(&b, ws("ContainerMode: %t,"), c.ContainerMode)
	case WindowsEventType:
		fmt.Fprintf(&b, ws("ChannelPath: %#v,"), c.ChannelPath)
		fmt.Fprintf(&b, ws("Query: %#v,"), c.Query)
	case StringChannelType:
		fmt.Fprintf(&b, ws("Channel: %p,"), c.Channel)
		c.ChannelTagsMutex.Lock()
		fmt.Fprintf(&b, ws("ChannelTags: %#v,"), c.ChannelTags)
		c.ChannelTagsMutex.Unlock()
	}
	fmt.Fprintf(&b, ws("Service: %#v,"), c.Service)
	fmt.Fprintf(&b, ws("Source: %#v,"), c.Source)
	fmt.Fprintf(&b, ws("SourceCategory: %#v,"), c.SourceCategory)
	fmt.Fprintf(&b, ws("Tags: %#v,"), c.Tags)
	fmt.Fprintf(&b, ws("ProcessingRules: %#v,"), c.ProcessingRules)
	if c.ProcessRawMessage != nil {
		fmt.Fprintf(&b, ws("ProcessRawMessage: %t,"), *c.ProcessRawMessage)
	} else {
		fmt.Fprint(&b, ws("ProcessRawMessage: nil,"))
	}
	fmt.Fprintf(&b, ws("ShouldProcessRawMessage(): %#v,"), c.ShouldProcessRawMessage())
	if c.AutoMultiLine != nil {
		fmt.Fprintf(&b, ws("AutoMultiLine: %t,"), *c.AutoMultiLine)
	} else {
		fmt.Fprint(&b, ws("AutoMultiLine: nil,"))
	}
	fmt.Fprintf(&b, ws("AutoMultiLineSampleSize: %d,"), c.AutoMultiLineSampleSize)
	fmt.Fprintf(&b, ws("AutoMultiLineMatchThreshold: %f}"), c.AutoMultiLineMatchThreshold)
	return b.String()
}

// PublicJSON serialize the structure to make sure we only export fields that can be relevant to customers.
// This is used to send the logs config to the backend as part of the metadata payload.
func (c *LogsConfig) PublicJSON() ([]byte, error) {
	// Export only fields that are explicitly documented in the public documentation
	return json.Marshal(&struct {
		Type            string            `json:"type,omitempty"`
		Port            int               `json:"port,omitempty"`           // Network
		Path            string            `json:"path,omitempty"`           // File, Journald
		Encoding        string            `json:"encoding,omitempty"`       // File
		ExcludePaths    []string          `json:"exclude_paths,omitempty"`  // File
		TailingMode     string            `json:"start_position,omitempty"` // File
		ChannelPath     string            `json:"channel_path,omitempty"`   // Windows Event
		Service         string            `json:"service,omitempty"`
		Source          string            `json:"source,omitempty"`
		Tags            []string          `json:"tags,omitempty"`
		ProcessingRules []*ProcessingRule `json:"log_processing_rules,omitempty"`
		AutoMultiLine   *bool             `json:"auto_multi_line_detection,omitempty"`
	}{
		Type:            c.Type,
		Port:            c.Port,
		Path:            c.Path,
		Encoding:        c.Encoding,
		ExcludePaths:    c.ExcludePaths,
		TailingMode:     c.TailingMode,
		ChannelPath:     c.ChannelPath,
		Service:         c.Service,
		Source:          c.Source,
		Tags:            c.Tags,
		ProcessingRules: c.ProcessingRules,
		AutoMultiLine:   c.AutoMultiLine,
	})
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
func (c *LogsConfig) AutoMultiLineEnabled(coreConfig pkgconfigmodel.Reader) bool {
	if c.AutoMultiLine != nil {
		return *c.AutoMultiLine
	}
	return coreConfig.GetBool("logs_config.auto_multi_line_detection")
}

// ShouldProcessRawMessage returns if the raw message should be processed instead
// of only the message content.
// This is tightly linked to how messages are transmitted through the pipeline.
// If returning true, tailers using structured message (journald, windowsevents)
// will fall back to original behavior of sending the whole message (e.g. JSON
// for journald) for post-processing.
// Otherwise, the message content is extracted from the structured message and
// only this part is post-processed and sent to the intake.
func (c *LogsConfig) ShouldProcessRawMessage() bool {
	if c.ProcessRawMessage != nil {
		return *c.ProcessRawMessage
	}
	return true // default behaviour when nothing's been configured
}

// ContainsWildcard returns true if the path contains any wildcard character
func ContainsWildcard(path string) bool {
	return strings.ContainsAny(path, "*?[")
}
