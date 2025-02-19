// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package settingsimpl

import (
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
	)
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp settings.Component
}

type mock struct {
	rtSettings map[string]interface{}
}

func newMock() MockProvides {
	m := mock{
		rtSettings: map[string]interface{}{},
	}
	return MockProvides{
		Comp: &m,
	}
}

// RuntimeSettings returns all runtime configurable settings
func (m mock) RuntimeSettings() map[string]settings.RuntimeSetting {
	return map[string]settings.RuntimeSetting{}
}

// GetRuntimeSetting returns the value of a runtime configurable setting
func (m mock) GetRuntimeSetting(key string) (interface{}, error) {
	v, found := m.rtSettings[key]
	if found {
		return v, nil
	}
	return nil, nil
}

// SetRuntimeSetting changes the value of a runtime configurable setting
func (m *mock) SetRuntimeSetting(key string, v interface{}, _ model.Source) error {
	m.rtSettings[key] = v
	return nil
}

// GetFullConfig returns the full config
func (m mock) GetFullConfig(...string) http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {}
}

// GetFullConfigBySource returns the full config by sources
func (m mock) GetFullConfigBySource() http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {}
}

// GetValue allows to retrieve the runtime setting
func (m mock) GetValue(http.ResponseWriter, *http.Request) {}

// SetValue allows to modify the runtime setting
func (m mock) SetValue(http.ResponseWriter, *http.Request) {}

// ListConfigurable returns the list of configurable setting at runtime
func (m mock) ListConfigurable(http.ResponseWriter, *http.Request) {}
