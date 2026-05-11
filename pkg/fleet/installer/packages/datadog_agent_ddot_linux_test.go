// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import "testing"

func TestDdotProcmgrYAMLBodyUsesStandaloneOCIPackage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"empty", "", false},
		{"extension_layout", "command: /opt/datadog-packages/datadog-agent/stable/ext/ddot/foo", false},
		{"standalone_layout", "command: /opt/datadog-packages/datadog-agent-ddot/stable/bin/otel-agent", true},
		{"condition_path_only", "condition_path_exists: /opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent", true},
		{"marker_only_in_description", "description: note datadog-packages/datadog-agent-ddot/stable\ncommand: /opt/datadog-packages/datadog-agent/stable/ext/ddot/x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ddotProcmgrYAMLBodyUsesStandaloneOCIPackage([]byte(tc.body)); got != tc.want {
				t.Fatalf("ddotProcmgrYAMLBodyUsesStandaloneOCIPackage(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
