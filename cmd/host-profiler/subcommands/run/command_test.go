// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package run

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/host-profiler/globalparams"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestFxRun tests that fx can build dependencies for the run command.
func TestFxRunWithoutAgentCore(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		MakeCommand(func() *globalparams.GlobalParams {
			return &globalparams.GlobalParams{ConfFilePath: "config_path"}
		}),
		[]string{"run"},
		run,
		func() {})
}

func TestFxRunWithAgentCore(t *testing.T) {
	// Use fxutil.TestOneShot as TestOneShotSubcommand would require valid datadog.yaml file, auth_token file and ipc_cert.pem.
	fxutil.TestOneShot(t, func() {
		runHostProfilerCommand(context.Background(), &cliParams{GlobalParams: &globalparams.GlobalParams{CoreConfPath: "config_path"}})
	})
}

func TestConfigStreamEnabled(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		enabled bool
	}{
		{name: "unset or empty", value: "", enabled: false},
		{name: "true", value: "true", enabled: true},
		{name: "false", value: "false", enabled: false},
		{name: "invalid", value: "not-a-bool", enabled: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(configStreamConsumerEnabledEnv, test.value)
			if enabled := isConfigStreamEnabled(); enabled != test.enabled {
				t.Fatalf("configStreamEnabled() = %t, want %t", enabled, test.enabled)
			}
		})
	}
}
