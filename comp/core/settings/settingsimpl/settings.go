// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package settingsimpl implements the interface for the settings component
package settingsimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newSettings),
	)
}

type provides struct {
	fx.Out

	Comp settings.Component
}

type dependencies struct {
	fx.In

	Log      log.Component
	Settings []settings.RuntimeSetting `group:"runtime_setting"`
}

type settingsRegistry struct {
	registeredSettings map[string]settings.RuntimeSetting
}

// RuntimeSettings returns all runtime configurable settings
func (s settingsRegistry) RuntimeSettings() map[string]settings.RuntimeSetting {
	return s.registeredSettings
}

// GetRuntimeSetting returns the value of a runtime configurable setting
func (s *settingsRegistry) GetRuntimeSetting(setting string) (interface{}, error) {
	if _, ok := s.registeredSettings[setting]; !ok {
		return nil, &settings.SettingNotFoundError{Name: setting}
	}
	value, err := s.registeredSettings[setting].Get()
	if err != nil {
		return nil, err
	}
	return value, nil
}

func newSettings(deps dependencies) provides {
	registeredSettings := map[string]settings.RuntimeSetting{}

	providedSettings := fxutil.GetAndFilterGroup(deps.Settings)

	for _, setting := range providedSettings {
		if _, ok := registeredSettings[setting.Name()]; ok {
			deps.Log.Warnf("duplicated settings detected: %s", setting.Name())
			continue
		}
		registeredSettings[setting.Name()] = setting
	}

	return provides{
		Comp: &settingsRegistry{
			registeredSettings: registeredSettings,
		},
	}
}
