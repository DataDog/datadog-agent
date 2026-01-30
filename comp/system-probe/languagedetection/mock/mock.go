// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the languagedetection component
package mock

import (
	"testing"

	languagedetection "github.com/DataDog/datadog-agent/comp/system-probe/languagedetection/def"
)

// Mock returns a mock for languagedetection component.
func Mock(t *testing.T) languagedetection.Component {
	// TODO: Implement the languagedetection mock
	return nil
}
