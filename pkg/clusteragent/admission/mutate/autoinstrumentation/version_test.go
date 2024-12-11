// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	testCases := []struct {
		name           string
		version        string
		expectsVersion version
		expectsErr     bool
		expectsPanic   bool
		usesInjector   bool
	}{
		{
			name:           "v1 is valid",
			version:        "v1",
			expectsVersion: instrumentationV1,
		},
		{
			name:           "v2 uses injector",
			version:        "v2",
			expectsVersion: instrumentationV2,
			usesInjector:   true,
		},
		{
			name:           "invalid version is invalid",
			version:        "something else",
			expectsVersion: instrumentationVersionInvalid,
			expectsErr:     true,
			expectsPanic:   true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			v, err := instrumentationVersion(tt.version)
			require.Equal(t, tt.expectsVersion, v)
			require.Equal(t, tt.expectsErr, err != nil)
			usesInjector := func() {
				require.Equal(t, tt.usesInjector, v.usesInjector())
			}
			if tt.expectsPanic {
				require.Panics(t, usesInjector)
			} else {
				require.NotPanics(t, usesInjector)
			}
		})

	}

}
