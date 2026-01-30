// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the softwareinventory component
package mock

import (
	"testing"

	softwareinventory "github.com/DataDog/datadog-agent/comp/system-probe/softwareinventory/def"
)

// Mock returns a mock for softwareinventory component.
func Mock(t *testing.T) softwareinventory.Component {
	// TODO: Implement the softwareinventory mock
	return nil
}
