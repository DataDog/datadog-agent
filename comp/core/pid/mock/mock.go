// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the pid component.
package mock

import (
	"testing"

	pid "github.com/DataDog/datadog-agent/comp/core/pid/def"
)

// Mock returns a mock for the pid component.
func Mock(_ *testing.T) pid.Component {
	return struct{}{}
}
