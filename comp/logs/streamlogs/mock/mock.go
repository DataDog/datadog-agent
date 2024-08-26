// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	streamlogs "github.com/DataDog/datadog-agent/comp/logs/streamlogs/def"
)

// Mock returns a mock for streamlogs component.
func Mock(t *testing.T) streamlogs.Component {
	// TODO: Implement the streamlogs mock
	return nil
}
