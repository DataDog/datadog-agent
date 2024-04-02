// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package settingsimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/settings"
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
func (m mock) RuntimeSettings() map[string]settings.RuntimeSetting {
	return map[string]settings.RuntimeSetting{}
}

// GetRuntimeSetting returns the value of a runtime configurable setting
func (m mock) GetRuntimeSetting(setting string) (interface{}, error) {
	return nil, nil
}
