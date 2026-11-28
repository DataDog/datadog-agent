// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package ssistatusimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

type fakeInventoryAgent struct {
	data map[string]any
}

func (f *fakeInventoryAgent) Set(name string, value any) {
	f.data[name] = value
}

func (f *fakeInventoryAgent) Get() map[string]any {
	return f.data
}

func (f *fakeInventoryAgent) Refresh() {}

func TestNewComponentPopulatesInventoryBeforeStart(t *testing.T) {
	ia := &fakeInventoryAgent{data: map[string]any{}}

	_, err := NewComponent(Requires{
		Lifecycle:      compdef.NewTestLifecycle(t),
		Log:            logmock.New(t),
		InventoryAgent: ia,
	})
	require.NoError(t, err)

	// Both keys must be populated before Lifecycle OnStart runs so the initial
	// agent_metadata payload sent at startup already reflects the host's SSI status.
	assert.Contains(t, ia.data, "feature_auto_instrumentation_enabled")
	assert.Contains(t, ia.data, "auto_instrumentation_modes")
}
