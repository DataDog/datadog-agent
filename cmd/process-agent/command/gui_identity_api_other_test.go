// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && test

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestShouldServeGUIIdentityAPI_Other(t *testing.T) {
	// Off Windows the GUI resolves peer identity in-process, so process-agent is never kept alive
	// for it — even with the GUI enabled.
	t.Run("GUI enabled still returns false", func(t *testing.T) {
		cfg := config.NewMockWithOverrides(t, map[string]interface{}{"GUI_port": 5002})
		assert.False(t, shouldServeGUIIdentityAPI(cfg))
	})

	t.Run("GUI disabled returns false", func(t *testing.T) {
		cfg := config.NewMockWithOverrides(t, map[string]interface{}{"GUI_port": -1})
		assert.False(t, shouldServeGUIIdentityAPI(cfg))
	})
}
