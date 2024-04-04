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
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
	)
}

type mock struct{}

func newMock() settings.Component {
	return mock{}
}

// RuntimeSettings returns all runtime configurable settings
func (m mock) RuntimeSettings() settings.Settings {
	return settings.Settings{}
}

// GetRuntimeSetting returns the value of a runtime configurable setting
func (m mock) GetRuntimeSetting(string) (interface{}, error) {
	return nil, nil
}

// SetRuntimeSetting changes the value of a runtime configurable setting
func (m mock) SetRuntimeSetting(string, interface{}, model.Source) error {
	return nil
}

// GetFullConfig returns the full config
func (m mock) GetFullConfig(config.Config, ...string) http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {}
}

// GetValue allows to retrieve the runtime setting
func (m mock) GetValue(string, http.ResponseWriter, *http.Request) {}

// SetValue allows to modify the runtime setting
func (m mock) SetValue(string, http.ResponseWriter, *http.Request) {}

// ListConfigurable returns the list of configurable setting at runtime
func (m mock) ListConfigurable(http.ResponseWriter, *http.Request) {}
