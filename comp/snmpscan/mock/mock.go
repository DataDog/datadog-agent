// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the snmpscan component
package mock

import (
	"testing"

	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
)

// Mock returns a mock for snmpscan component.
func Mock(t *testing.T) snmpscan.Component {
	// TODO: Implement the snmpscan mock
	return nil
}
