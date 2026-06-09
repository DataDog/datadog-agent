// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package configmock provides mocks for the config component.
package configmock

import (
	"testing"

	configdef "github.com/DataDog/datadog-agent/comp/core/config/def"
	configimpl "github.com/DataDog/datadog-agent/comp/core/config/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule provides a mock config component via fx.
// Works with both fxutil.Test and fxutil.TestApp.
// If testing.TB is available in the fx container (fxutil.Test), it is used for
// proper test cleanup; otherwise a no-op is used.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			func() configdef.Component {
				return New(noopTB{})
			},
		),
	)
}

// noopTB is a minimal testing.TB that ignores Cleanup calls.
// Used by MockModule so the mock works without a real test context.
type noopTB struct{ testing.TB }

func (noopTB) Cleanup(func()) {}

// New returns a mock for the config component.
func New(t testing.TB) configdef.Component {
	return configimpl.NewCfgFromPkgConfig(mock.New(t))
}

// NewWithOverrides creates a mock config and calls SetInTest on every item in overrides.
func NewWithOverrides(t testing.TB, overrides map[string]interface{}) configdef.Component {
	conf := mock.New(t)
	for k, v := range overrides {
		conf.SetInTest(k, v)
	}
	return configimpl.NewCfgFromPkgConfig(conf)
}

// NewFromYAML returns a mock for the config component with the given YAML content loaded into it.
func NewFromYAML(t testing.TB, yaml string) configdef.Component {
	return configimpl.NewCfgFromPkgConfig(mock.NewFromYAML(t, yaml))
}

// NewFromYAMLFile returns a mock for the config component with the given YAML file loaded into it.
func NewFromYAMLFile(t testing.TB, yamlFilePath string) configdef.Component {
	return configimpl.NewCfgFromPkgConfig(mock.NewFromFile(t, yamlFilePath))
}
