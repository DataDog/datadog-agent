// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"io"
	"strings"
	"time"

	"github.com/DataDog/viper"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
)

// Proxy represents the configuration for proxies in the agent
type Proxy struct {
	HTTP    string   `mapstructure:"http"`
	HTTPS   string   `mapstructure:"https"`
	NoProxy []string `mapstructure:"no_proxy"`
}

// ConfigReader is a subset of Config that only allows reading of configuration
type ConfigReader interface {
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

	ConfigFileUsed() string

	AllSettings() map[string]interface{}
	AllSettingsWithoutDefault() map[string]interface{}
	AllKeys() []string

	IsSet(key string) bool

	// IsKnown returns whether this key is known
	IsKnown(key string) bool

	// GetKnownKeys returns all the keys that meet at least one of these criteria:
	// 1) have a default, 2) have an environment variable binded, 3) are an alias or 4) have been SetKnown()
	GetKnownKeys() map[string]interface{}

	// GetEnvVars returns a list of the env vars that the config supports.
	// These have had the EnvPrefix applied, as well as the EnvKeyReplacer.
	GetEnvVars() []string

	// IsSectionSet checks if a given section is set by checking if any of
	// its subkeys is set.
	IsSectionSet(section string) bool
}

type ConfigWriter interface {
	Set(key string, value interface{})
	CopyConfig(cfg Config)
}

type ConfigReaderWriter interface {
	ConfigReader
	ConfigWriter
}

type ConfigLoader interface {
	// API implemented by viper.Viper

	SetDefault(key string, value interface{})
	SetFs(fs afero.Fs)

	SetEnvPrefix(in string)
	BindEnv(input ...string)
	SetEnvKeyReplacer(r *strings.Replacer)
	SetEnvKeyTransformer(key string, fn func(string) interface{})

	UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error
	Unmarshal(rawVal interface{}) error
	UnmarshalExact(rawVal interface{}) error

	ReadInConfig() error
	ReadConfig(in io.Reader) error
	MergeConfig(in io.Reader) error
	MergeConfigOverride(in io.Reader) error

	AddConfigPath(in string)
	SetConfigName(in string)
	SetConfigFile(in string)
	SetConfigType(in string)

	BindPFlag(key string, flag *pflag.Flag) error

	// SetKnown adds a key to the set of known valid config keys
	SetKnown(key string)

	// API not implemented by viper.Viper and that have proven useful for our config usage

	// BindEnvAndSetDefault sets the default value for a config parameter and adds an env binding
	// in one call, used for most config options.
	//
	// If env is provided, it will override the name of the environment variable used for this
	// config key
	BindEnvAndSetDefault(key string, val interface{}, env ...string)
}

// Config represents an object that can load and store configuration parameters
// coming from different kind of sources:
// - defaults
// - files
// - environment variables
// - flags
type Config interface {
	ConfigReaderWriter
	ConfigLoader
}
