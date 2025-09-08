// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the privateactionrunner component
package mock

import (
	"context"
	"testing"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
)

type mock struct {
	Logger log.Component
}

// Start implements the privateactionrunner.Component interface
func (m mock) Start(_ context.Context) error {
	return nil
}

// Stop implements the privateactionrunner.Component interface
func (m mock) Stop(_ context.Context) error {
	return nil
}

// GetRunnerID implements the privateactionrunner.Component interface
func (m mock) GetRunnerID() string {
	return "" // Mock returns empty string by default
}

// Provides that defines the output of mocked privateactionrunner component
type Provides struct {
	comp privateactionrunner.Component
}

// New returns a mock privateactionrunner
func New(*testing.T) Provides {
	return Provides{
		comp: mock{},
	}
}
