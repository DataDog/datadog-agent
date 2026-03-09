// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && windows

// Package mock provides a mock for the systray component.
package mock

import (
	"testing"

	systray "github.com/DataDog/datadog-agent/comp/systray/systray/def"
)

// Mock returns a mock for the systray component.
func Mock(_ *testing.T) systray.Component {
	return struct{}{}
}
