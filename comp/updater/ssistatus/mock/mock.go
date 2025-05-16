// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the ssistatus component
package mock

import (
	"testing"

	ssistatus "github.com/DataDog/datadog-agent/comp/updater/ssistatus/def"
)

// Mock returns a mock for ssistatus component.
func Mock(_ *testing.T) ssistatus.Component {
	return nil
}
