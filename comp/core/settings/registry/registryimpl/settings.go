// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package registryimpl implements the interface for the settings component
package registryimpl

import (
	"sync"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/settings/registry"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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

	Comp registry.Component
}

type dependencies struct {
	fx.In

	Log      log.Component
	Settings []registry.RuntimeSetting `group:"runtime_setting"`
}

type settingsRegistry struct {
	m                  sync.Mutex
	registeredSettings map[string]registry.RuntimeSetting
}

// RuntimeSettings returns all runtime configurable settings
func (s settingsRegistry) RuntimeSettings() map[string]registry.RuntimeSetting {
	return s.registeredSettings
}

// GetRuntimeSetting returns the value of a runtime configurable setting
func (s *settingsRegistry) GetRuntimeSetting(setting string) (interface{}, error) {
	if _, ok := s.registeredSettings[setting]; !ok {
		return nil, &registry.SettingNotFoundError{Name: setting}
	}
	value, err := s.registeredSettings[setting].Get()
	if err != nil {
		return nil, err
	}
	return value, nil
}

// SetRuntimeSetting changes the value of a runtime configurable setting
func (s *settingsRegistry) SetRuntimeSetting(setting string, value interface{}, source model.Source) error {
	s.m.Lock()
	defer s.m.Unlock()
	if _, ok := s.registeredSettings[setting]; !ok {
		return &registry.SettingNotFoundError{Name: setting}
	}
	return s.registeredSettings[setting].Set(value, source)
}

func newSettings(deps dependencies) provides {
	registeredSettings := map[string]registry.RuntimeSetting{}

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
