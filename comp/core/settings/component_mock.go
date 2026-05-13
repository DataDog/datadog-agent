// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package settings provides mock-specific types for the settings component.
package settings

import settingsdef "github.com/DataDog/datadog-agent/comp/core/settings/def"

// Mock implements mock-specific methods.
type Mock interface {
	settingsdef.Component
}
