// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// configless is a Handler that renders no configs at all.
type configless struct{}

func (h *configless) Key() string { return "example" }

func (h *configless) Render(_ string, _ json.RawMessage) ([]integration.Config, error) {
	return nil, nil
}

func TestHandlerSupportsAnImplementationThatSchedulesNothing(t *testing.T) {
	var h Handler = &configless{}

	assert.Equal(t, "example", h.Key())

	configs, err := h.Render("datadog/2/PRODUCT/id/config", json.RawMessage(`{"a":1}`))
	assert.NoError(t, err)
	assert.Nil(t, configs)
}
