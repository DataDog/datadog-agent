// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the privilegedlogs component
package mock

import (
	"testing"

	privilegedlogs "github.com/DataDog/datadog-agent/comp/system-probe/privilegedlogs/def"
)

// Mock returns a mock for privilegedlogs component.
func Mock(t *testing.T) privilegedlogs.Component {
	// TODO: Implement the privilegedlogs mock
	return nil
}
