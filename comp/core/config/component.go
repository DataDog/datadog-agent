// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements a component to handle agent configuration.  This
// component temporarily wraps pkg/config.
//
// This component initializes pkg/config based on the bundle params, and
// will return the same results as that package.  This is to support migration
// to a component architecture.  When no code still uses pkg/config, that
// package will be removed.
//
// The mock component does nothing at startup, beginning with an empty config.
// It also overwrites the pkg/config.Datadog for the duration of the test.
package config

import (
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// IsSet determines whether the given config parameter is set.
	//
	// This includes values set in the config file, by environment variables, or with
	// a default value.
	IsSet(key string) bool

	// Get gets the underlying value of a config parameter, without conversion.
	Get(key string) interface{}

	// GetString gets a string-typed config parameter value.
	GetString(key string) string

	// GetBool gets an integer-typed config parameter value.
	GetBool(key string) bool

	// GetInt gets an integer-typed config parameter value.
	GetInt(key string) int

	// GetInt32 gets an int32 config parameter value.
	GetInt32(key string) int32

	// GetInt64 gets an int64 config parameter value.
	GetInt64(key string) int64

	// GetFloat64 gets an float64 config parameter value.
	GetFloat64(key string) float64

	// GetTime gets a time as a config parameter value.
	GetTime(key string) time.Time

	// GetTime gets a duration as a config parameter value, parsing common suffixes.
	// Bare integers are treated as nanoseconds.
	GetDuration(key string) time.Duration

	// GetTime gets a slice of strings (represented as a list in the source YAML)
	GetStringSlice(key string) []string

	// GetTime gets a slice of floats (represented as a list in the source
	// YAML) as a config parameter value, returning an error if any of the
	// values cannot be parsed
	GetFloat64SliceE(key string) ([]float64, error)

	// GetStringMap gets a map (represented as an object in the source YAML) as
	// a config parameter value.
	GetStringMap(key string) map[string]interface{}

	// GetStringMapString gets a map (represented as an object in the source
	// YAML) as a config parameter value, treating each value as a string.
	GetStringMapString(key string) map[string]string

	// GetStringMapStringSlice gets a map (represented as an object in the source
	// YAML) as a config parameter value, treating each value as a string slice
	// (represented as an array in the source YAML).
	GetStringMapStringSlice(key string) map[string][]string

	// GetSizeInBytes gets a size as a config parameter value, parsing common suffixes.
	GetSizeInBytes(key string) uint

	// AllSettings merges all settings and returns them as a map[string]interface{}.
	AllSettings() map[string]interface{}

	// AllSettingsWithoutDefault is like AllSettings but omits configuration that was
	// applied based on defaults.
	AllSettingsWithoutDefault() map[string]interface{}

	// AllKeys returns all keys holding a value, regardless of where they are
	// set. Nested keys are returned with a v.keyDelim separator
	AllKeys() []string

	// GetKnownKeys returns all the keys that meet at least one of these criteria:
	// 1) have a default, 2) have an environment variable binded, 3) are an alias or 4) have been SetKnown()
	GetKnownKeys() map[string]interface{}

	// GetEnvVars returns a list of the env vars that the config supports.
	// These have had the EnvPrefix applied, as well as the EnvKeyReplacer.
	GetEnvVars() []string

	// IsSectionSet checks if a given section is set by checking if any of
	// its subkeys is set.
	IsSectionSet(section string) bool

	// Warnings returns config warnings collected during setup.
	Warnings() *config.Warnings
}

// Mock implements mock-specific methods.
type Mock interface {
	Component

	// Set sets the given config value
	Set(key string, value interface{})
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newConfig),
)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
)
