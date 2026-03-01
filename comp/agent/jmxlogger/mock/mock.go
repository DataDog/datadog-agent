// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the jmxlogger component
package mock

import (
	"testing"

	jmxlogger "github.com/DataDog/datadog-agent/comp/agent/jmxlogger/def"
)

type mockJMXLogger struct{}

func (m *mockJMXLogger) JMXInfo(_ ...interface{})        {}
func (m *mockJMXLogger) JMXError(_ ...interface{}) error { return nil }
func (m *mockJMXLogger) Flush()                          {}

// Mock returns a mock for the jmxlogger component.
func Mock(_ *testing.T) jmxlogger.Component {
	return &mockJMXLogger{}
}
