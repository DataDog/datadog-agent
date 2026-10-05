// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package issueregistryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

// No issue modules are imported here, so GetAllModules returns an empty slice.
// These tests verify that NewComponent() wires the inner registry correctly and that all
// Component methods delegate to it without panicking on an empty registry.

func TestNewReturnsValidComponent(t *testing.T) {
	comp := NewComponent(Requires{Config: config.NewMock(t)})
	assert.NotNil(t, comp)
}

func TestGetTemplateReturnsFalseForUnknown(t *testing.T) {
	comp := NewComponent(Requires{Config: config.NewMock(t)})
	_, ok := comp.GetTemplate("unknown-type")
	assert.False(t, ok)
}

func TestGetBuiltInPeriodicHealthChecksEmptyRegistry(t *testing.T) {
	comp := NewComponent(Requires{Config: config.NewMock(t)})
	checks := comp.GetBuiltInPeriodicHealthChecks()
	require.NotNil(t, checks)
	assert.Empty(t, checks)
}

func TestGetBuiltInStartupHealthChecksEmptyRegistry(t *testing.T) {
	comp := NewComponent(Requires{Config: config.NewMock(t)})
	checks := comp.GetBuiltInStartupHealthChecks()
	require.NotNil(t, checks)
	assert.Empty(t, checks)
}
