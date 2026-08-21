// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package dogstatsdcommon

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cconfig "github.com/DataDog/datadog-agent/comp/core/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestCheckDataPlaneOwnsDogstatsd(t *testing.T) {
	tests := []struct {
		name      string
		setupCfg  func(cfg cconfig.Component)
		setEnv    string // if non-empty, sets DD_ADP_ENABLED to this value
		wantOwned bool
	}{
		{
			name: "data plane enabled for dogstatsd",
			setupCfg: func(cfg cconfig.Component) {
				cfg.SetInTest("data_plane.enabled", true)
				cfg.SetInTest("data_plane.dogstatsd.enabled", true)
			},
			wantOwned: true,
		},
		{
			name: "data plane enabled but dogstatsd disabled",
			setupCfg: func(cfg cconfig.Component) {
				cfg.SetInTest("data_plane.enabled", true)
				cfg.SetInTest("data_plane.dogstatsd.enabled", false)
			},
			wantOwned: false,
		},
		{
			name: "use_dogstatsd false overrides data plane",
			setupCfg: func(cfg cconfig.Component) {
				cfg.SetInTest("use_dogstatsd", false)
				cfg.SetInTest("data_plane.enabled", true)
				cfg.SetInTest("data_plane.dogstatsd.enabled", true)
			},
			wantOwned: false,
		},
		{
			name:      "deprecated DD_ADP_ENABLED env var",
			setupCfg:  func(_ cconfig.Component) {},
			setEnv:    "true",
			wantOwned: true,
		},
		{
			name:      "defaults (no data plane, no env var)",
			setupCfg:  func(_ cconfig.Component) {},
			wantOwned: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv != "" {
				t.Setenv("DD_ADP_ENABLED", tc.setEnv)
			} else {
				t.Setenv("DD_ADP_ENABLED", "")
			}

			cfg := configmock.New(t)
			tc.setupCfg(cfg)

			err := CheckDataPlaneOwnsDogstatsd(cfg)
			if tc.wantOwned {
				assert.ErrorIs(t, err, ErrDataPlaneOwnsDogstatsd)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
