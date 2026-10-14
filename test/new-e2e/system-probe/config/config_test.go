// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"maps"
	"testing"
)

func TestTestEnvVars(t *testing.T) {
	tests := []struct {
		name          string
		arch          string
		kernelRelease string
		expected      map[string]string
	}{
		{
			name:          "fentry supported on amazon 5.10 x86_64",
			arch:          "x86_64",
			kernelRelease: "5.10.244-240.970.amzn2",
			expected:      map[string]string{"DD_KMT_NETWORK_TRACER_FENTRY_SUPPORTED": "true"},
		},
		{
			name:          "fentry not supported on amazon 5.10 arm64",
			arch:          "arm64",
			kernelRelease: "5.10.244-240.970.amzn2",
			expected:      map[string]string{"DD_KMT_NETWORK_TRACER_FENTRY_SUPPORTED": "false"},
		},
		{
			name:          "fentry not supported on fedora 37",
			arch:          "x86_64",
			kernelRelease: "6.0.7-301.fc37",
			expected:      map[string]string{"DD_KMT_NETWORK_TRACER_FENTRY_SUPPORTED": "false"},
		},
		{
			name:          "fentry supported on ubuntu 25.10",
			arch:          "arm64",
			kernelRelease: "6.17.0-5-generic",
			expected:      map[string]string{"DD_KMT_NETWORK_TRACER_FENTRY_SUPPORTED": "true"},
		},
		{
			name:          "no env vars for unknown (custom) kernels",
			arch:          "x86_64",
			kernelRelease: "6.99.0-custom",
			expected:      map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			envVars, err := TestEnvVars(tc.arch, tc.kernelRelease)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !maps.Equal(envVars, tc.expected) {
				t.Errorf("expected %v, got %v", tc.expected, envVars)
			}
		})
	}
}

// TestAllPlatformsDefineFentryEnvVar ensures new platforms are not silently
// added to platforms.json without deciding whether fentry is supported.
func TestAllPlatformsDefineFentryEnvVar(t *testing.T) {
	plats, err := parsePlatforms()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for arch, byArch := range map[string]map[string]platform{"x86_64": plats.X8664, "arm64": plats.Arm64} {
		for tag, plat := range byArch {
			if _, ok := plat.TestEnvVars["DD_KMT_NETWORK_TRACER_FENTRY_SUPPORTED"]; !ok {
				t.Errorf("%s/%s does not define the DD_KMT_NETWORK_TRACER_FENTRY_SUPPORTED test env var", arch, tag)
			}
		}
	}
}
