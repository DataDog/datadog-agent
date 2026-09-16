// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the settings component
package mock

import (
	"net/http"

	settingsdef "github.com/DataDog/datadog-agent/comp/core/settings/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(NewMockComponent),
	)
}

// MockProvides is the mock component output.
type MockProvides struct {
	compdef.Out

	Comp settingsdef.Component
}

type mockSettings struct {
	rtSettings map[string]interface{}
}

// NewMockComponent creates a mock settings component.
func NewMockComponent() MockProvides {
	m := mockSettings{
		rtSettings: map[string]interface{}{},
	}
	return MockProvides{
		Comp: &m,
	}
}

// RuntimeSettings returns all runtime configurable settings
func (m mockSettings) RuntimeSettings() map[string]settingsdef.RuntimeSetting {
	return map[string]settingsdef.RuntimeSetting{}
}

// GetRuntimeSetting returns the value of a runtime configurable setting
func (m mockSettings) GetRuntimeSetting(key string) (interface{}, error) {
	v, found := m.rtSettings[key]
	if found {
		return v, nil
	}
	return nil, nil
}

// SetRuntimeSetting changes the value of a runtime configurable setting
func (m *mockSettings) SetRuntimeSetting(key string, v interface{}, _ model.Source) error {
	m.rtSettings[key] = v
	return nil
}

// GetFullConfig returns the full config
func (m mockSettings) GetFullConfig(...string) http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {}
}

// GetFullConfigWithoutDefaults returns the full config without defaults
func (m mockSettings) GetFullConfigWithoutDefaults(...string) http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {}
}

// GetFullConfigBySource returns the full config by sources
func (m mockSettings) GetFullConfigBySource() http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {}
}

// GetValue allows to retrieve the runtime setting
func (m mockSettings) GetValue(http.ResponseWriter, *http.Request) {}

// SetValue allows to modify the runtime setting
func (m mockSettings) SetValue(http.ResponseWriter, *http.Request) {}

// ListConfigurable returns the list of configurable setting at runtime
func (m mockSettings) ListConfigurable(http.ResponseWriter, *http.Request) {}
