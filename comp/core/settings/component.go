// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package settings defines the interface for the component that manage settings that can be changed at runtime
package settings

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"go.uber.org/fx"
)

// team: agent-shared-components

// SettingNotFoundError is used to warn about non existing/not registered runtime setting
type SettingNotFoundError struct {
	Name string
}

func (e *SettingNotFoundError) Error() string {
	return fmt.Sprintf("setting %s not found", e.Name)
}

// RuntimeSettingResponse is used to communicate settings config
type RuntimeSettingResponse struct {
	Description string
	Hidden      bool
}

// Settings define the runtime settings the component would understand
type Settings map[string]RuntimeSetting

// Component is the component type.
type Component interface {
	// RuntimeSettings returns the configurable settings
	RuntimeSettings() Settings
	// GetRuntimeSetting returns the value of a runtime configurable setting
	GetRuntimeSetting(setting string) (interface{}, error)
	// SetRuntimeSetting changes the value of a runtime configurable setting
	SetRuntimeSetting(setting string, value interface{}, source model.Source) error

	// API related functions
	// Todo: (Components) Remove these functions once we can register routes using FX value groups
	// GetFullConfig returns the full config
	GetFullConfig(cfg config.Config, namespaces ...string) http.HandlerFunc
	// GetValue allows to retrieve the runtime setting
	GetValue(setting string, w http.ResponseWriter, r *http.Request)
	// SetValue allows to modify the runtime setting
	SetValue(setting string, w http.ResponseWriter, r *http.Request)
	// ListConfigurable returns the list of configurable setting at runtime
	ListConfigurable(w http.ResponseWriter, r *http.Request)
}

// RuntimeSetting represents a setting that can be changed and read at runtime.
type RuntimeSetting interface {
	Get() (interface{}, error)
	Set(v interface{}, source model.Source) error
	Description() string
	Hidden() bool
}

// RuntimeSettingProvider stores the Provider instance
type RuntimeSettingProvider struct {
	fx.Out

	Setting RuntimeSetting `group:"runtime_setting"`
}

// NewRuntimeSettingProvider returns a RuntimeSettingProvider
func NewRuntimeSettingProvider(runtimesetting RuntimeSetting) RuntimeSettingProvider {
	return RuntimeSettingProvider{
		Setting: runtimesetting,
	}
}
