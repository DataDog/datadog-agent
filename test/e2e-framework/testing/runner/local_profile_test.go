// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package runner

import (
	"testing"
)

func TestNamePrefix(t *testing.T) {
	tests := []struct {
		name      string
		realUser  string
		workspace string
		want      string
	}{
		{
			name:     "real user collapses first.last",
			realUser: "john.doe",
			want:     "jdoe",
		},
		{
			name:      "workspace name disambiguates two workspaces of the same developer",
			realUser:  "john.doe",
			workspace: "test-e2e",
			want:      "jdoe-test-e2e",
		},
		{
			name:      "workspace name is sanitized",
			realUser:  "john.doe",
			workspace: "Test E2E",
			want:      "jdoe-test-e2e",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("REAL_USER", tt.realUser)
			t.Setenv("WORKSPACE_NAME", tt.workspace)

			if got := (localProfile{}).NamePrefix(); got != tt.want {
				t.Errorf("localProfile.NamePrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Without REAL_USER the prefix must keep coming from the OS user, so laptops are
// unaffected by the workspace support.
func TestNamePrefixWithoutRealUser(t *testing.T) {
	t.Setenv("REAL_USER", "")
	t.Setenv("WORKSPACE_NAME", "")

	if got := (localProfile{}).NamePrefix(); got == "" {
		t.Error("localProfile.NamePrefix() = \"\", want the sanitized OS user")
	}
}
