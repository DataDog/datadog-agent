// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package settings defines the interface for the component that manage settings that can be changed at runtime
package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"go.uber.org/fx"
)

// team: agent-shared-components

// SettingNotFoundError is used to warn about non existing/not registered runtime setting
type SettingNotFoundError struct {
	Name string
}

// RuntimeSettingResponse is used to communicate settings config
type RuntimeSettingResponse struct {
	Description string
	Hidden      bool
}

func (e *SettingNotFoundError) Error() string {
	return fmt.Sprintf("setting %s not found", e.Name)
}

// Component is the component type.
type Component interface {
	// RuntimeSettings returns the configurable settings
	RuntimeSettings() map[string]RuntimeSetting
	// GetRuntimeSetting returns the value of a runtime configurable setting
	GetRuntimeSetting(setting string) (interface{}, error)
}

// RuntimeSetting represents a setting that can be changed and read at runtime.
type RuntimeSetting interface {
	Get() (interface{}, error)
	Set(v interface{}, source model.Source) error
	Name() string
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
