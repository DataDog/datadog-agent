// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock implements a streamlogs component to be used in tests
package mock

import (
	streamlogs "github.com/DataDog/datadog-agent/comp/logs/streamlogs/def"
)

// Mock returns a mock for streamlogs component.
func Mock() streamlogs.Component {
	return nil
}
