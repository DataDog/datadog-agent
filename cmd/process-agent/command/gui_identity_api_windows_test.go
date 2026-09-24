// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestShouldServeGUIIdentityAPI_Windows(t *testing.T) {
	t.Run("GUI enabled keeps process-agent alive for the peer-identity API", func(t *testing.T) {
		cfg := config.NewMockWithOverrides(t, map[string]interface{}{"GUI_port": 5002})
		assert.True(t, shouldServeGUIIdentityAPI(cfg))
	})

	t.Run("GUI enabled on a non-default port still keeps it alive", func(t *testing.T) {
		cfg := config.NewMockWithOverrides(t, map[string]interface{}{"GUI_port": 5010})
		assert.True(t, shouldServeGUIIdentityAPI(cfg))
	})

	t.Run("GUI disabled (-1 sentinel) does not keep it alive", func(t *testing.T) {
		cfg := config.NewMockWithOverrides(t, map[string]interface{}{"GUI_port": -1})
		assert.False(t, shouldServeGUIIdentityAPI(cfg))
	})
}
