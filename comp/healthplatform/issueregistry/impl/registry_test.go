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

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

// An empty issue group yields an empty registry; all component methods must delegate safely.

func TestNewReturnsValidComponent(t *testing.T) {
	comp := NewComponent(Requires{Log: logmock.New(t)})
	assert.NotNil(t, comp)
}

func TestGetTemplateReturnsFalseForUnknown(t *testing.T) {
	comp := NewComponent(Requires{Log: logmock.New(t)})
	_, ok := comp.GetTemplate("unknown-type")
	assert.False(t, ok)
}

func TestGetBuiltInPeriodicHealthChecksEmptyRegistry(t *testing.T) {
	comp := NewComponent(Requires{Log: logmock.New(t)})
	checks := comp.GetBuiltInPeriodicHealthChecks()
	require.NotNil(t, checks)
	assert.Empty(t, checks)
}

func TestGetBuiltInStartupHealthChecksEmptyRegistry(t *testing.T) {
	comp := NewComponent(Requires{Log: logmock.New(t)})
	checks := comp.GetBuiltInStartupHealthChecks()
	require.NotNil(t, checks)
	assert.Empty(t, checks)
}
