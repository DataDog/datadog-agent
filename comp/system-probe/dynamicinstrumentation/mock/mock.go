// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the dynamicinstrumentation component
package mock

import (
	"testing"

	dynamicinstrumentation "github.com/DataDog/datadog-agent/comp/system-probe/dynamicinstrumentation/def"
)

// Mock returns a mock for dynamicinstrumentation component.
func Mock(_t *testing.T) dynamicinstrumentation.Component {
	// TODO: Implement the dynamicinstrumentation mock
	return nil
}
