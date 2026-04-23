// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package settings defines the interface for the component that manage settings that can be changed at runtime
package settings

import (
	settingsdef "github.com/DataDog/datadog-agent/comp/core/settings/def"
)

// team: agent-configuration

// Component is the component type.
// Deprecated: use comp/core/settings/def instead.
type Component = settingsdef.Component

// RuntimeSetting represents a setting that can be changed and read at runtime.
// Deprecated: use comp/core/settings/def instead.
type RuntimeSetting = settingsdef.RuntimeSetting

// SettingNotFoundError is used to warn about non existing/not registered runtime setting
// Deprecated: use comp/core/settings/def instead.
type SettingNotFoundError = settingsdef.SettingNotFoundError

// RuntimeSettingResponse is used to communicate settings config
// Deprecated: use comp/core/settings/def instead.
type RuntimeSettingResponse = settingsdef.RuntimeSettingResponse

// Params that the settings component need
// Deprecated: use comp/core/settings/def instead.
type Params = settingsdef.Params

// RuntimeSettingProvider stores the Provider instance
// Deprecated: use comp/core/settings/def instead.
type RuntimeSettingProvider = settingsdef.RuntimeSettingProvider

// NewRuntimeSettingProvider returns a RuntimeSettingProvider
// Deprecated: use comp/core/settings/def instead.
var NewRuntimeSettingProvider = settingsdef.NewRuntimeSettingProvider
