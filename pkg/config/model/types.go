// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"io"
	"strings"
	"time"

	"github.com/DataDog/viper"
)

// Proxy represents the configuration for proxies in the agent
type Proxy struct {
	HTTP    string   `mapstructure:"http"`
	HTTPS   string   `mapstructure:"https"`
	NoProxy []string `mapstructure:"no_proxy"`
}

// NotificationReceiver represents the callback type to receive notifications each time the `Set` method is called. The
// configuration will call each NotificationReceiver registered through the 'OnUpdate' method, therefore
// 'NotificationReceiver' should not be blocking.
type NotificationReceiver func(setting string, oldValue, newValue any)

// Reader is a subset of Config that only allows reading of configuration
type Reader interface {
	Get(key string) interface{}
	GetString(key string) string
	GetBool(key string) bool
	GetInt(key string) int
	GetInt32(key string) int32
	GetInt64(key string) int64
	GetFloat64(key string) float64
	GetTime(key string) time.Time
	GetDuration(key string) time.Duration
	GetStringSlice(key string) []string
	GetFloat64SliceE(key string) ([]float64, error)
	GetStringMap(key string) map[string]interface{}
	GetStringMapString(key string) map[string]string
	GetStringMapStringSlice(key string) map[string][]string
	GetSizeInBytes(key string) uint
	GetProxies() *Proxy

	GetSource(key string) Source
	GetAllSources(key string) []ValueWithSource

	ConfigFileUsed() string
	ExtraConfigFilesUsed() []string

	AllSettings() map[string]interface{}
	AllSettingsWithoutDefault() map[string]interface{}
	AllSettingsBySource() map[Source]interface{}
	// AllKeysLowercased returns all config keys in the config, no matter how they are set.
	// Note that it returns the keys lowercased.
	AllKeysLowercased() []string

	IsSet(key string) bool

	// UnmarshalKey Unmarshal a configuration key into a struct
	UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error

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
}

// Writer is a subset of Config that only allows writing the configuration
type Writer interface {
	Set(key string, value interface{}, source Source)
	SetWithoutSource(key string, value interface{})
	UnsetForSource(key string, source Source)
	CopyConfig(cfg Config)
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
	UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error

	ReadInConfig() error
	ReadConfig(in io.Reader) error
	MergeConfig(in io.Reader) error
	MergeConfigMap(cfg map[string]any) error
	MergeFleetPolicy(configPath string) error
}

// Config represents an object that can load and store configuration parameters
// coming from different kind of sources:
// - defaults
// - files
// - environment variables
// - flags
type Config interface {
	ReaderWriter
	Setup
	Compound
}
