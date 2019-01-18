package types

import (
	"regexp"
)

type LogSources interface {
	AddSource(source *LogSource)
	RemoveSource(source *LogSource)
	GetAddedForType(sourceType string) chan *LogSource
	GetRemovedForType(sourceType string) chan *LogSource
	GetSources() []LogSource
}

type LogSource interface {
	AddInput(input string)
	RemoveInput(input string)
	GetInputs() []string
	SetSourceType(sourceType string)
	GetSourceType() string
	GetName() string
	GetConfig() LogsConfig
	GetStatus() LogStatus
	GetMessages() *Messages
}

type LogStatus interface {
	Success()
	Error(err error)
	IsPending() bool
	IsSuccess() bool
	IsError() bool
	GetError() string
}

// Logs source types
const (
	TCPType          = "tcp"
	UDPType          = "udp"
	FileType         = "file"
	ContainerdType   = "containerd"
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
type LogsConfig interface {
	Validate() error
	Compile() error

	GetType() string

	GetPort() int    // Network
	GetPath() string // File, Journald

	GetIncludeUnits() []string // Journald
	GetExcludeUnits() []string // Journald

	GetImage() string      // Docker
	GetLabel() string      // Docker
	GetName() string       // Docker
	GetIdentifier() string // Docker

	GetChannelPath() string // Windows Event
	GetQuery() string       // Windows Event

	GetService() string
	GetSource() string
	GetSourceCategory() string
	GetTags() []string
	GetProcessingRules() []ProcessingRule
}
