// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package containermode

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t, Commands(&command.GlobalParams{}), []string{"resolve-container-mode"}, run, func() {})
}

func TestWriteMode(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		split   bool
		want    string
	}{
		{name: "disabled", split: true, want: monolithicMode},
		{name: "monolithic", enabled: true, want: monolithicMode},
		{name: "split", enabled: true, split: true, want: splitMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"private_action_runner.enabled":       tt.enabled,
				"private_action_runner.split_enabled": tt.split,
			})
			var out bytes.Buffer

			require.NoError(t, writeMode(cfg, &out))
			require.Equal(t, tt.want+"\n", out.String())
		})
	}
}
