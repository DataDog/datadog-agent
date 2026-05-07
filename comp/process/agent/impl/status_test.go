// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package agentimpl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	procstatus "github.com/DataDog/datadog-agent/pkg/process/status"
)

func TestStatus(t *testing.T) {
	env.SetFeatures(t)

	cfg := config.NewMock(t)
	cfg.SetWithoutSource("hostname", "test-host")

	// Seed the package-level state that GetInProcessStatus reads.
	procstatus.InitExpvars(cfg, "test-host", true, true, nil)

	provider := StatusProvider{}

	t.Run("JSON", func(t *testing.T) {
		stats := make(map[string]interface{})
		require := assert.New(t)
		require.NoError(provider.JSON(false, stats))

		raw, ok := stats["processComponentStatus"].(map[string]interface{})
		require.True(ok)
		require.Empty(raw["error"])
		require.NotEmpty(raw["core"])

		core, ok := raw["core"].(map[string]interface{})
		require.True(ok)
		require.NotEmpty(core["version"])
		require.NotEmpty(core["go_version"])
	})

	t.Run("Text", func(t *testing.T) {
		b := new(bytes.Buffer)
		require := assert.New(t)
		require.NoError(provider.Text(false, b))
		// The template is gated on "if .error"; absence of "Not running"
		// confirms we took the success branch.
		require.False(strings.Contains(b.String(), "Not running or unreachable"))
	})

	t.Run("HTML", func(t *testing.T) {
		b := new(bytes.Buffer)
		require := assert.New(t)
		require.NoError(provider.HTML(false, b))
		require.Empty(b.String())
	})
}
