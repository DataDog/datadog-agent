// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	mapstructure "github.com/go-viper/mapstructure/v2"
)

// ErrConfigFileNotFound is an error for when the config file is not found
var ErrConfigFileNotFound = errors.New("Config File Not Found")

// NewConfigFileNotFoundError returns a well known error for the config file missing
func NewConfigFileNotFoundError(err error) error {
	return fmt.Errorf("%w: %w", ErrConfigFileNotFound, err)
}

// Source stores what edits a setting as a string
type Source string

// Declare every known Source
const (
	// SourceSchema are settings define in the schema for the configuration but without any default.
	SourceSchema Source = "schema"
	// SourceDefault are the values from defaults.
	SourceDefault Source = "default"
	// SourceUnknown are the values from unknown source. This should only be used in tests when calling
	// SetWithoutSource.
	SourceUnknown Source = "unknown"
	// SourceFile are the values loaded from configuration file.
	SourceFile Source = "file"
	// SourceEnvVar are the values loaded from the environment variables.
	SourceEnvVar Source = "environment-variable"
	// SourceAgentRuntime are the values configured by the agent itself. The agent can dynamically compute the best
	// value for some settings when not set by the user.
	SourceAgentRuntime Source = "agent-runtime"
	// SourceLocalConfigProcess are the values mirrored from the config process. The config process is the
	// core-agent. This is used when side process like security-agent or trace-agent pull their configuration from
	// the core-agent.
	SourceLocalConfigProcess Source = "local-config-process"
	// SourceRC are the values loaded from remote-config (aka Datadog backend)
	SourceRC Source = "remote-config"
	// SourceFleetPolicies are the values loaded from remote-config file
	SourceFleetPolicies Source = "fleet-policies"
	// SourceCLI are the values set by the user at runtime through the CLI.
	SourceCLI Source = "cli"
	// SourceProvided are all values set by any source but default.
	SourceProvided Source = "provided" // everything but defaults
)

// Sources list the known sources, following the order of hierarchy between them
var Sources = []Source{
	SourceDefault,
	SourceUnknown,
	SourceFile,
	SourceEnvVar,
	SourceFleetPolicies,
	SourceAgentRuntime,
	SourceLocalConfigProcess,
	SourceRC,
	SourceCLI,
}

// sourcesPriority give each source a priority, the higher the more important a source. This is used when merging
// configuration tree (a higher priority overwrites a lower one).
var sourcesPriority = map[Source]int{
	SourceSchema:             -1,
	SourceDefault:            0,
	SourceUnknown:            1,
	SourceFile:               2,
	SourceEnvVar:             3,
	SourceFleetPolicies:      4,
	SourceAgentRuntime:       5,
	SourceLocalConfigProcess: 6,
	SourceRC:                 7,
	SourceCLI:                8,
}

// ValueWithSource is a tuple for a source and a value, not necessarily the applied value in the main config
type ValueWithSource struct {
	Source Source
	Value  interface{}
}

// IsGreaterThan returns true if the current source is of higher priority than the one given as a parameter
func (s Source) IsGreaterThan(x Source) bool {
	return sourcesPriority[s] > sourcesPriority[x]
}

// PreviousSource returns the source before the current one, or Default (lowest priority) if there isn't one
func (s Source) PreviousSource() Source {
	previous := sourcesPriority[s]
	if previous == 0 {
		return Sources[previous]
	}
	return Sources[previous-1]
}

// String casts Source into a string
func (s Source) String() string {
	// Safeguard: if we don't know the Source, we assume SourceUnknown
	if s == "" {
		return string(SourceUnknown)
	}
	return string(s)
}

// Proxy represents the configuration for proxies in the agent
type Proxy struct {
	HTTP    string   `mapstructure:"http"`
	HTTPS   string   `mapstructure:"https"`
	NoProxy []string `mapstructure:"no_proxy"`
}

// NotificationReceiver represents the callback type to receive notifications each time the `Set` method is called. The
// configuration will call each NotificationReceiver registered through the 'OnUpdate' method, therefore
// 'NotificationReceiver' should not be blocking.
type NotificationReceiver func(setting string, source Source, oldValue, newValue any, sequenceID uint64)

// Reader is a subset of Config that only allows reading of configuration
type Reader interface {
	Get(key string) interface{}
	GetString(key string) string
	GetBool(key string) bool
	GetInt(key string) int
	GetInt32(key string) int32
	GetInt64(key string) int64
	GetFloat64(key string) float64
	GetDuration(key string) time.Duration
	GetStringSlice(key string) []string
	GetFloat64Slice(key string) []float64
	GetStringMap(key string) map[string]interface{}
	GetStringMapString(key string) map[string]string
	GetStringMapStringSlice(key string) map[string][]string
	GetSizeInBytes(key string) uint
	GetProxies() *Proxy
	GetSequenceID() uint64

	GetSource(key string) Source
	GetAllSources(key string) []ValueWithSource
	GetSubfields(key string) []string

	ConfigFileUsed() string
	ExtraConfigFilesUsed() []string

	AllSettings() map[string]interface{}
	AllSettingsWithoutDefault() map[string]interface{}
	AllSettingsBySource() map[Source]interface{}
	// AllKeysLowercased returns all config keys in the config, no matter how they are set.
	// Note that it returns the keys lowercased.
	AllKeysLowercased() []string
	AllSettingsWithSequenceID() (map[string]interface{}, uint64)

	// SetTestOnlyDynamicSchema is used by tests to disable validation of the config schema
	// This lets tests use the config is more flexible ways (can add to the schema at any point,
	// can modify env vars and the config will rebuild itself, etc)
	SetTestOnlyDynamicSchema(allow bool)

	// IsSet return true if a non nil values is found in the configuration, including defaults. This is legacy
	// behavior from viper and don't answer the need to know if something was set by the user (see IsConfigured for
	// this).
	//
	// Deprecated: this method will be removed once all settings have a default, use 'IsConfigured' instead.
	IsSet(key string) bool
	// IsConfigured returns true if a setting exists, has a value and doesn't come from the defaults (ie: was
	// configured by the user). If a setting is configured by the user with the same value than the defaults this
	// method will still return true as it tests the source of a setting not its value.
	IsConfigured(key string) bool

	// UnmarshalKey Unmarshal a configuration key into a struct
	UnmarshalKey(key string, rawVal interface{}, opts ...func(*mapstructure.DecoderConfig)) error

	// IsKnown returns whether this key is known
	IsKnown(key string) bool

	// GetKnownKeysLowercased returns all the keys that meet at least one of these criteria:
	// 1) have a default, 2) have an environment variable binded, 3) are an alias or 4) have been SetKnown()
	// Note that it returns the keys lowercased.
	GetKnownKeysLowercased() map[string]interface{}

	// GetEnvVars returns a list of the env vars that the config supports.
	// These have had the EnvPrefix applied, as well as the EnvKeyReplacer.
	GetEnvVars() []string

	// Warnings returns pointer to a list of warnings (completes config.Component interface)
	Warnings() *Warnings

	// Object returns Reader to config (completes config.Component interface)
	Object() Reader

	// OnUpdate adds a callback to the list receivers to be called each time a value is change in the configuration
	// by a call to the 'Set' method. The configuration will sequentially call each receiver.
	OnUpdate(callback NotificationReceiver)

	// Stringify stringifies the config, only available if "test" build tag is enabled
	Stringify(source Source, opts ...StringifyOption) string
}

// Writer is a subset of Config that only allows writing the configuration
type Writer interface {
	Set(key string, value interface{}, source Source)
	SetWithoutSource(key string, value interface{})
	UnsetForSource(key string, source Source)
}

// ReaderWriter is a subset of Config that allows reading and writing the configuration
type ReaderWriter interface {
	Reader
	Writer
}

// Setup is a subset of Config that allows setting up the configuration
type Setup interface {
	// API implemented by viper.Viper

	// BuildSchema should be called when Setup is done, it builds the schema making the config ready for use
	BuildSchema()

	SetDefault(key string, value interface{})

	SetEnvPrefix(in string)
	BindEnv(key string, envvars ...string)
	SetEnvKeyReplacer(r *strings.Replacer)

	// The following helpers allow a type to be enforce when parsing environment variables. Most of them exists to
	// support historic behavior. Refrain from adding more as it's most likely a sign of poorly design configuration
	// layout.
	ParseEnvAsStringSlice(key string, fx func(string) []string)
	ParseEnvAsMapStringInterface(key string, fx func(string) map[string]interface{})
	ParseEnvAsSliceMapString(key string, fx func(string) []map[string]string)
	ParseEnvAsSlice(key string, fx func(string) []interface{})

	// SetKnown adds a key to the set of known valid config keys
	SetKnown(key string)

	// API not implemented by viper.Viper and that have proven useful for our config usage

	// BindEnvAndSetDefault sets the default value for a config parameter and adds an env binding
	// in one call, used for most config options.
	//
	// If env is provided, it will override the name of the environment variable used for this
	// config key
	BindEnvAndSetDefault(key string, val interface{}, env ...string)

	AddConfigPath(in string)
	AddExtraConfigPaths(in []string) error
	SetConfigName(in string)
	SetConfigFile(in string)
	SetConfigType(in string)
}

// Compound is an interface for retrieving compound elements from the config, plus
// some misc functions, that should likely be split into another interface
type Compound interface {
	UnmarshalKey(key string, rawVal interface{}, opts ...func(*mapstructure.DecoderConfig)) error

	ReadInConfig() error
	ReadConfig(in io.Reader) error
	MergeConfig(in io.Reader) error
	MergeFleetPolicy(configPath string) error

	// Revert a finished configuration so that more can be build on top of it.
	// When building is completed, the caller should call BuildSchema.
	// NOTE: This method should not be used by any new callsites, it is needed
	// currently because of the unique requirements of OTel's configuration.
	RevertFinishedBackToBuilder() BuildableConfig
}

// Config is an interface that can read/write the config after it has been
// build and initialized.
type Config interface {
	ReaderWriter
	Compound
	// TODO: This method shouldn't be here, but it is depended upon by an external repository
	// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e7c3295769637e61558c6892be732398840dd5f5/pkg/datadog/agentcomponents/agentcomponents.go#L166
	SetKnown(key string)
}

// BuildableConfig is the most-general interface for the Config, it can be
// used both to build the config and also to read/write its values. It should
// only be used when necessary, such as when constructing a new config object
// from scratch.
type BuildableConfig interface {
	ReaderWriter
	Setup
	Compound
}
