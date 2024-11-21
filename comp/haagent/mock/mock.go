// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the haagent component
package mock

import (
	"testing"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
)

type mock struct {
	Logger log.Component
}

func (m *mock) GetGroup() string {
	return "mockGroup01"
}

func (m *mock) Enabled() bool {
	return true
}

func (m *mock) SetLeader(_ string) {
}

func (m *mock) IsLeader() bool { return false }

// Provides that defines the output of mocked snmpscan component
type Provides struct {
	comp haagent.Component
}

// Mock returns a mock for haagent component.
func Mock(_ *testing.T) Provides {
	return Provides{
		comp: &mock{},
	}
}
