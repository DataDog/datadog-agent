// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the apiobserver component
package mock

import (
	"testing"

	apiobserver "github.com/DataDog/datadog-agent/comp/api/apiobserver/def"
)

// Mock returns a mock for apiobserver component.
func Mock(t *testing.T) apiobserver.Component {
	// TODO: Implement the apiobserver mock
	return nil
}
