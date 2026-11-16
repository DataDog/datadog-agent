// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package deployment

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
)

func TestResolve(t *testing.T) {
	detectionErr := errors.New("FIPS detection failed")
	for _, tc := range []struct {
		name, goos, envValue                                           string
		disabled, containerized, buildFIPS, configFIPS, processManager bool
		fipsErr                                                        error
		want                                                           Mode
		probe                                                          bool
	}{
		{name: "disabled", goos: "linux", disabled: true, fipsErr: detectionErr, want: Monolithic},
		{name: "linux host", goos: "linux", want: Split, probe: true},
		{name: "host ignores direct environment override", goos: "linux", envValue: "false", want: Split, probe: true},
		{name: "linux container explicit env", goos: "linux", containerized: true, envValue: "true", disabled: true, want: Split, probe: true},
		{name: "container config alone is insufficient", goos: "linux", containerized: true, fipsErr: detectionErr, want: Monolithic},
		{name: "container explicit off", goos: "linux", containerized: true, envValue: "false", want: Monolithic},
		{name: "container env is case sensitive", goos: "linux", containerized: true, envValue: "TRUE", want: Monolithic},
		{name: "windows host", goos: "windows", processManager: true, want: Split, probe: true},
		{name: "windows requires process manager", goos: "windows", want: Monolithic},
		{name: "windows container", goos: "windows", processManager: true, containerized: true, envValue: "true", want: Monolithic},
		{name: "unsupported host platform", goos: "darwin", want: Monolithic},
		{name: "build FIPS", goos: "linux", buildFIPS: true, want: Monolithic, probe: true},
		{name: "configured FIPS", goos: "linux", configFIPS: true, fipsErr: detectionErr, want: Monolithic},
		{name: "FIPS detection error", goos: "linux", fipsErr: detectionErr, want: Monolithic, probe: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.New(t)
			cfg.SetInTest(par.SplitEnabled, !tc.disabled)
			cfg.SetInTest("fips.enabled", tc.configFIPS)
			cfg.SetInTest("process_manager.enabled", tc.processManager)
			calls := 0
			mode, err := resolve(cfg, tc.goos, tc.containerized, tc.envValue, func() (bool, error) {
				calls++
				return tc.buildFIPS, tc.fipsErr
			})
			require.Equal(t, tc.want, mode)
			if tc.probe {
				require.Equal(t, 1, calls)
				require.ErrorIs(t, err, tc.fipsErr)
			} else {
				require.Zero(t, calls, "unused split mode must not probe FIPS")
				require.NoError(t, err)
			}
		})
	}
}
