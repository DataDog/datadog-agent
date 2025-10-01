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
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	IdleTimeout string `mapstructure:"idle_timeout" json:"idle_timeout" yaml:"idle_timeout"` // Network
	Path        string // File, Journald

	Encoding     string           `mapstructure:"encoding" json:"encoding" yaml:"encoding"`                   // File
	ExcludePaths StringSliceField `mapstructure:"exclude_paths" json:"exclude_paths" yaml:"exclude_paths"`    // File
	TailingMode  string           `mapstructure:"start_position" json:"start_position" yaml:"start_position"` // File

	ConfigID           string           `mapstructure:"config_id" json:"config_id" yaml:"config_id"`                            // Journald
	IncludeSystemUnits StringSliceField `mapstructure:"include_units" json:"include_units" yaml:"include_units"`                // Journald
	ExcludeSystemUnits StringSliceField `mapstructure:"exclude_units" json:"exclude_units" yaml:"exclude_units"`                // Journald
	IncludeUserUnits   StringSliceField `mapstructure:"include_user_units" json:"include_user_units" yaml:"include_user_units"` // Journald
	ExcludeUserUnits   StringSliceField `mapstructure:"exclude_user_units" json:"exclude_user_units" yaml:"exclude_user_units"` // Journald
	IncludeMatches     StringSliceField `mapstructure:"include_matches" json:"include_matches" yaml:"include_matches"`          // Journald
	ExcludeMatches     StringSliceField `mapstructure:"exclude_matches" json:"exclude_matches" yaml:"exclude_matches"`          // Journald
	ContainerMode      bool             `mapstructure:"container_mode" json:"container_mode" yaml:"container_mode"`             // Journald

	Image string // Docker
	Label string // Docker
	// Name contains the container name
	Name string // Docker
	// Identifier contains the container ID.  This is also set for File sources and used to
	// determine the appropriate tags for the logs.
	Identifier string // Docker, File

	ChannelPath string `mapstructure:"channel_path" json:"channel_path" yaml:"channel_path"` // Windows Event
	Query       string // Windows Event

	// used as input only by the Channel tailer.
	// could have been unidirectional but the tailer could not close it in this case.
	Channel chan *ChannelMessage

	// ChannelTags are the tags attached to messages on Channel; unlike Tags this can be
	// modified at runtime (as long as ChannelTagsMutex is held).
	ChannelTags StringSliceField

	// ChannelTagsMutex guards ChannelTags.
	ChannelTagsMutex sync.Mutex

	Service         string
	Source          string
	SourceCategory  string
	Tags            StringSliceField
	ProcessingRules []*ProcessingRule `mapstructure:"log_processing_rules" json:"log_processing_rules" yaml:"log_processing_rules"`
	// ProcessRawMessage is used to process the raw message instead of only the content part of the message.
	ProcessRawMessage *bool `mapstructure:"process_raw_message" json:"process_raw_message" yaml:"process_raw_message"`

	AutoMultiLine               *bool   `mapstructure:"auto_multi_line_detection" json:"auto_multi_line_detection" yaml:"auto_multi_line_detection"`
	AutoMultiLineSampleSize     int     `mapstructure:"auto_multi_line_sample_size" json:"auto_multi_line_sample_size" yaml:"auto_multi_line_sample_size"`
	AutoMultiLineMatchThreshold float64 `mapstructure:"auto_multi_line_match_threshold" json:"auto_multi_line_match_threshold" yaml:"auto_multi_line_match_threshold"`
	// AutoMultiLineOptions provides detailed configuration for auto multi-line detection specific to this source.
	// It maps to the 'auto_multi_line' key in the YAML configuration.
	AutoMultiLineOptions *SourceAutoMultiLineOptions `mapstructure:"auto_multi_line" json:"auto_multi_line" yaml:"auto_multi_line"`
	// CustomSamples holds the raw string content of the 'auto_multi_line_detection_custom_samples' YAML block.
	// Downstream code will be responsible for parsing this string.
	AutoMultiLineSamples []*AutoMultilineSample   `mapstructure:"auto_multi_line_detection_custom_samples" json:"auto_multi_line_detection_custom_samples" yaml:"auto_multi_line_detection_custom_samples"`
	FingerprintConfig    *types.FingerprintConfig `mapstructure:"fingerprint_config" json:"fingerprint_config" yaml:"fingerprint_config"`

	// IntegrationSource is the source of the integration file that contains this source.
	IntegrationSource string `mapstructure:"integration_source" json:"integration_source" yaml:"integration_source"`
	// IntegrationFileIndex is the index of the integration file that contains this source.
	IntegrationSourceIndex int `mapstructure:"integration_source_index" json:"integration_source_index" yaml:"integration_source_index"`
}

// SourceAutoMultiLineOptions defines per-source auto multi-line detection overrides.
// These settings allow for fine-grained control over auto multi-line detection
// for a specific log source, potentially overriding global configurations.
type SourceAutoMultiLineOptions struct {
	// EnableJSONDetection allows to enable or disable the detection of multi-line JSON logs for this source.
	EnableJSONDetection *bool `mapstructure:"enable_json_detection" json:"enable_json_detection" yaml:"enable_json_detection"`

	// EnableDatetimeDetection allows to enable or disable the detection of multi-lines based on leading datetime stamps for this source.
	EnableDatetimeDetection *bool `mapstructure:"enable_datetime_detection" json:"enable_datetime_detection" yaml:"enable_datetime_detection"`

	// MatchThreshold sets the similarity threshold to consider a pattern match for this source.
	TimestampDetectorMatchThreshold *float64 `mapstructure:"timestamp_detector_match_threshold" json:"timestamp_detector_match_threshold" yaml:"timestamp_detector_match_threshold"`

	// TokenizerMaxInputBytes sets the maximum number of bytes the tokenizer will read for this source.
	TokenizerMaxInputBytes *int `mapstructure:"tokenizer_max_input_bytes" json:"tokenizer_max_input_bytes" yaml:"tokenizer_max_input_bytes"`

	// PatternTableMaxSize sets the number of patterns auto multi line can use
	PatternTableMaxSize *int `mapstructure:"pattern_table_max_size" json:"pattern_table_max_size" yaml:"pattern_table_max_size"`

	// PatternTableMatchThreshold sets the threshold for pattern table match for this source.
	PatternTableMatchThreshold *float64 `mapstructure:"pattern_table_match_threshold" json:"pattern_table_match_threshold" yaml:"pattern_table_match_threshold"`

	// EnableJSONAggregation allows to enable or disable the aggregation of multi-line JSON logs for this source.
	EnableJSONAggregation *bool `mapstructure:"enable_json_aggregation" json:"enable_json_aggregation" yaml:"enable_json_aggregation"`

	// TagAggregatedJSON allows to enable or disable the tagging of aggregated JSON logs for this source.
	TagAggregatedJSON *bool `mapstructure:"tag_aggregated_json" json:"tag_aggregated_json" yaml:"tag_aggregated_json"`
}

// AutoMultilineSample defines a sample used to create auto multiline detection
// rules
type AutoMultilineSample struct {
	// Sample is a raw log message sample used to aggregate logs.
	Sample string `mapstructure:"sample" json:"sample" yaml:"sample"`
	// MatchThreshold is the ratio of tokens that must match between the sample and the log message to consider it a match.
	// From a user perspective, this is how similar the log has to be to the sample to be considered a match.
	// Optional - Default value is 0.75.
	MatchThreshold *float64 `mapstructure:"match_threshold,omitempty" json:"match_threshold,omitempty"`
	// Regex is a pattern used to aggregate logs. NOTE that you can use either a sample or a regex, but not both.
	Regex string `mapstructure:"regex,omitempty" json:"regex,omitempty"`
	// Label is the label to apply to the log message if it matches the sample.
	// Optional - Default value is "start_group".
	Label *string `mapstructure:"label,omitempty" json:"label,omitempty"`
}

// StringSliceField is a custom type for unmarshalling comma-separated string values or typical yaml fields into a slice of strings.
type StringSliceField []string

// UnmarshalYAML is a custom unmarshalling function is needed for string array fields to split comma-separated values.
func (t *StringSliceField) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err == nil {
		// note that we are intentionally avoiding the trimming of any spaces whilst splitting the string
		str = strings.ReplaceAll(str, "\n", "")
		*t = strings.Split(str, ",")
		return nil
	}

	var raw []interface{}
	if err := unmarshal(&raw); err == nil {
		for _, item := range raw {
			if str, ok := item.(string); ok {
				*t = append(*t, str)
			} else {
				return fmt.Errorf("cannot unmarshal %v into a string", item)
			}
		}
		return nil
	}
	return fmt.Errorf("could not parse YAML config, please double check the yaml files")
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
		fmt.Fprintf(&b, ws("TailingMode: %#v,"), c.TailingMode)
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
	fmt.Fprintf(&b, ws("AutoMultiLineMatchThreshold: %f,"), c.AutoMultiLineMatchThreshold)
	if c.FingerprintConfig != nil {
		fmt.Fprintf(&b, ws("FingerprintConfig: %+v}"), c.FingerprintConfig)
	} else {
		fmt.Fprint(&b, ws("FingerprintConfig: nil}"))
	}
	return b.String()
}

// PublicJSON serialize the structure to make sure we only export fields that can be relevant to customers.
// This is used to send the logs config to the backend as part of the metadata payload.
func (c *LogsConfig) PublicJSON() ([]byte, error) {
	// Export only fields that are explicitly documented in the public documentation
	return json.Marshal(&struct {
		Type              string                   `json:"type,omitempty"`
		Port              int                      `json:"port,omitempty"`           // Network
		Path              string                   `json:"path,omitempty"`           // File, Journald
		Encoding          string                   `json:"encoding,omitempty"`       // File
		ExcludePaths      []string                 `json:"exclude_paths,omitempty"`  // File
		TailingMode       string                   `json:"start_position,omitempty"` // File
		ChannelPath       string                   `json:"channel_path,omitempty"`   // Windows Event
		Service           string                   `json:"service,omitempty"`
		Source            string                   `json:"source,omitempty"`
		Tags              []string                 `json:"tags,omitempty"`
		ProcessingRules   []*ProcessingRule        `json:"log_processing_rules,omitempty"`
		AutoMultiLine     *bool                    `json:"auto_multi_line_detection,omitempty"`
		FingerprintConfig *types.FingerprintConfig `json:"fingerprint_config,omitempty"`
	}{
		Type:              c.Type,
		Port:              c.Port,
		Path:              c.Path,
		Encoding:          c.Encoding,
		ExcludePaths:      c.ExcludePaths,
		TailingMode:       c.TailingMode,
		ChannelPath:       c.ChannelPath,
		Service:           c.Service,
		Source:            c.Source,
		Tags:              c.Tags,
		ProcessingRules:   c.ProcessingRules,
		AutoMultiLine:     c.AutoMultiLine,
		FingerprintConfig: c.FingerprintConfig,
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

	// Validate fingerprint configuration
	err := ValidateFingerprintConfig(c.FingerprintConfig)
	if err != nil {
		return err
	}

	err = ValidateProcessingRules(c.ProcessingRules)
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

// LegacyAutoMultiLineEnabled determines whether the agent has fallen back to legacy auto multi line detection
// for compatibility reasons.
func (c *LogsConfig) LegacyAutoMultiLineEnabled(coreConfig pkgconfigmodel.Reader) bool {

	// Handle explicit user initiated fallback to V1
	if coreConfig.GetBool("logs_config.force_auto_multi_line_detection_v1") {
		log.Info("Auto multi line detection falling back to legacy mode for log source:", c.Source, "because the force_auto_multi_line_detection_v1 is set to true.")
		return c.AutoMultiLineEnabled(coreConfig)
	}

	// Handle transparent fallback if V1 was explicitly configured.
	if c.AutoMultiLineSampleSize != 0 || coreConfig.IsConfigured("logs_config.auto_multi_line_default_sample_size") {
		log.Warn("Auto multi line detection falling back to legacy mode for log source:", c.Source, "because the sample size has been set to a non-default value.")
		return c.AutoMultiLineEnabled(coreConfig)
	}
	if c.AutoMultiLineMatchThreshold != 0 || coreConfig.IsConfigured("logs_config.auto_multi_line_default_match_threshold") {
		log.Warn("Auto multi line detection falling back to legacy mode for log source:", c.Source, "because the match threshold has been set to a non-default value.")
		return c.AutoMultiLineEnabled(coreConfig)
	}

	if coreConfig.IsConfigured("logs_config.auto_multi_line_default_match_timeout") {
		log.Warn("Auto multi line detection falling back to legacy mode for log source:", c.Source, "because the match timeout has been set to a non-default value.")
		return c.AutoMultiLineEnabled(coreConfig)
	}
	return false
}

// AutoMultiLineEnabled determines whether auto multi line detection is enabled for this config,
// considering both the agent-wide logs_config.auto_multi_line_detection and any config for this
// particular log source.
func (c *LogsConfig) AutoMultiLineEnabled(coreConfig pkgconfigmodel.Reader) bool {
	if c.AutoMultiLine != nil {
		return *c.AutoMultiLine
	}
	if coreConfig.GetBool("logs_config.experimental_auto_multi_line_detection") {
		log.Warn("logs_config.experimental_auto_multi_line_detection is deprecated, use logs_config.auto_multi_line_detection instead")
		return true
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
