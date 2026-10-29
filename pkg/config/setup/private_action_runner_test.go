// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package setup

import (
	"errors"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

func TestPrivateActionRunnerDefault(t *testing.T) {
	for _, tc := range []struct {
		name, goos, launch                                       string
		containerized, noManager, noSplit, configFIPS, buildFIPS bool
		fipsErr                                                  error
		want, probe                                              bool
	}{
		{name: "packaged split", goos: "linux", launch: "true", want: true, probe: true},
		{name: "unmarked legacy or plain systemd", goos: "linux"},
		{name: "marker requires exact true", goos: "linux", launch: "TRUE"},
		{name: "manager opted out", goos: "linux", launch: "true", noManager: true},
		{name: "monolithic selected", goos: "linux", launch: "true", noSplit: true},
		{name: "container with host process definitions", goos: "linux", launch: "true", containerized: true},
		{name: "configured FIPS", goos: "linux", launch: "true", configFIPS: true},
		{name: "build FIPS", goos: "linux", launch: "true", buildFIPS: true, probe: true},
		{name: "FIPS detection error", goos: "linux", launch: "true", fipsErr: errors.New("detection failed"), probe: true},
		{name: "Windows unchanged", goos: "windows", want: true},
		{name: "macOS unchanged", goos: "darwin", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConf(t)
			cfg.Set("private_action_runner.enabled", true, model.SourceDefault)
			cfg.Set("private_action_runner.split_enabled", !tc.noSplit, model.SourceFile)
			cfg.Set("process_manager.enabled", !tc.noManager, model.SourceFile)
			cfg.Set("fips.enabled", tc.configFIPS, model.SourceFile)
			probed := false
			setPrivateActionRunnerDefault(cfg, tc.goos, tc.containerized, tc.launch, func() (bool, error) {
				probed = true
				return tc.buildFIPS, tc.fipsErr
			})
			require.Equal(t, tc.want, cfg.GetBool("private_action_runner.enabled"))
			require.Equal(t, model.SourceDefault, cfg.GetSource("private_action_runner.enabled"))
			require.Equal(t, tc.probe, probed)
		})
	}
}

func TestPrivateActionRunnerDefaultPreservesExplicitSettings(t *testing.T) {
	for _, source := range []model.Source{model.SourceFile, model.SourceEnvVar, model.SourceFleetPolicies, model.SourceCLI} {
		for _, enabled := range []bool{false, true} {
			for _, launch := range []string{"", "true"} {
				t.Run(fmt.Sprintf("%s/enabled=%t/launch=%s", source, enabled, launch), func(t *testing.T) {
					if source == model.SourceEnvVar {
						t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", fmt.Sprint(enabled))
					}
					cfg := newTestConf(t)
					if source != model.SourceEnvVar {
						cfg.Set("private_action_runner.enabled", enabled, source)
					}
					cfg.Set("private_action_runner.split_enabled", true, model.SourceFile)
					cfg.Set("process_manager.enabled", true, model.SourceFile)
					setPrivateActionRunnerDefault(cfg, "linux", false, launch, func() (bool, error) { return false, nil })
					require.Equal(t, enabled, cfg.GetBool("private_action_runner.enabled"))
					require.Equal(t, source, cfg.GetSource("private_action_runner.enabled"))
				})
			}
		}
	}
}

func TestPrivateActionRunnerDefaultRecomputesAfterPolicyChanges(t *testing.T) {
	cfg := newTestConf(t)
	cfg.Set("private_action_runner.split_enabled", true, model.SourceFile)
	cfg.Set("process_manager.enabled", true, model.SourceFile)
	apply := func() {
		setPrivateActionRunnerDefault(cfg, "linux", false, "true", func() (bool, error) { return false, nil })
	}
	apply()
	require.True(t, cfg.GetBool("private_action_runner.enabled"))
	cfg.Set("process_manager.enabled", false, model.SourceFleetPolicies)
	apply()
	require.False(t, cfg.GetBool("private_action_runner.enabled"))
	cfg.Set("process_manager.enabled", true, model.SourceFleetPolicies)
	apply()
	require.True(t, cfg.GetBool("private_action_runner.enabled"))
	cfg.Set("private_action_runner.split_enabled", false, model.SourceCLI)
	apply()
	require.False(t, cfg.GetBool("private_action_runner.enabled"))
}
