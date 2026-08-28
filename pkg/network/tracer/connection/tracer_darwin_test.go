// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestDarwinTracerBackendSelection(t *testing.T) {
	for _, tc := range []struct {
		name           string
		backend        string
		nstatError     error
		wantNStatCalls int
		wantPcapCalls  int
		wantError      bool
	}{
		{
			name:          "default",
			wantPcapCalls: 1,
		},
		{
			name:          "ebpfless",
			backend:       config.DarwinConnectionTracerEbpfless,
			wantPcapCalls: 1,
		},
		{
			name:           "nstat",
			backend:        config.DarwinConnectionTracerNStat,
			wantNStatCalls: 1,
		},
		{
			name:           "nstat with packet enrichment",
			backend:        config.DarwinConnectionTracerNStatPcap,
			wantNStatCalls: 1,
		},
		{
			name:           "auto",
			backend:        config.DarwinConnectionTracerAuto,
			wantNStatCalls: 1,
		},
		{
			name:           "nstat fallback",
			backend:        config.DarwinConnectionTracerNStat,
			nstatError:     errors.New("unavailable"),
			wantNStatCalls: 1,
			wantPcapCalls:  1,
		},
		{
			name:      "unknown",
			backend:   "invalid",
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var nstatCalls, pcapCalls int
			_, err := newDarwinTracer(
				&config.Config{DarwinConnectionTracerBackend: tc.backend},
				func() (Tracer, error) {
					pcapCalls++
					return nil, nil
				},
				func() (Tracer, error) {
					nstatCalls++
					return nil, tc.nstatError
				},
			)

			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantNStatCalls, nstatCalls)
			require.Equal(t, tc.wantPcapCalls, pcapCalls)
		})
	}
}
