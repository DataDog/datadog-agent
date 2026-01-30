// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the ebpf component
package mock

import (
	"testing"

	ebpf "github.com/DataDog/datadog-agent/comp/system-probe/ebpf/def"
)

// Mock returns a mock for ebpf component.
func Mock(t *testing.T) ebpf.Component {
	// TODO: Implement the ebpf mock
	return nil
}
