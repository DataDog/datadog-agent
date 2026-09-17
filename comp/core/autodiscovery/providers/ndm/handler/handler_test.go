// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// configless is a Renderer that renders no configs at all.
type configless struct{}

func (h *configless) Key() string { return "example" }

func (h *configless) Render(_ string, _ json.RawMessage) ([]integration.Config, error) {
	return nil, nil
}

// scheduling is a Snapshotter that applies every path it is given.
type scheduling struct{}

func (h *scheduling) Key() string { return "scheduling" }

func (h *scheduling) Snapshot(_ map[string]json.RawMessage) map[string]error { return nil }

func TestRendererSupportsAnImplementationThatSchedulesNothing(t *testing.T) {
	var h Renderer = &configless{}

	assert.Equal(t, "example", h.Key())

	configs, err := h.Render("datadog/2/PRODUCT/id/config", json.RawMessage(`{"a":1}`))
	assert.NoError(t, err)
	assert.Nil(t, configs)
}

func TestSnapshotterIsAHandler(t *testing.T) {
	var h Handler = &scheduling{}

	assert.Equal(t, "scheduling", h.Key())

	snapshotter, ok := h.(Snapshotter)
	require.True(t, ok)
	assert.Empty(t, snapshotter.Snapshot(map[string]json.RawMessage{"path-a": json.RawMessage(`{}`)}))
}
