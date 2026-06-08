// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides mock for configstreamconsumer component
package mock

import (
	"testing"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
)

// Mock is a mock implementation of configstreamconsumer.Component
type Mock struct {
	t *testing.T
}

// New creates a new mock configstreamconsumer component
func New(t *testing.T) configstreamconsumer.Component {
	return &Mock{t: t}
}
