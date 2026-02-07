// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the tcpqueuelength component
package mock

import (
	"testing"

	tcpqueuelength "github.com/DataDog/datadog-agent/comp/system-probe/tcpqueuelength/def"
)

// Mock returns a mock for tcpqueuelength component.
func Mock(_t *testing.T) tcpqueuelength.Component {
	// TODO: Implement the tcpqueuelength mock
	return nil
}
