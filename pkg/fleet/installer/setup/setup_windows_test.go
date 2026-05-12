// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package setup

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

func TestResolveAgentOCITag(t *testing.T) {
	cases := []struct {
		name  string
		major string
		minor string
		want  string
	}{
		{"unset", "", "", "latest"},
		{"bare minor", "", "78", "7.78"},
		{"explicit patch", "", "78.0", "7.78.0-1"},
		{"explicit patch with release suffix", "", "78.0-1", "7.78.0-1"},
		{"tilde RC", "", "78.0~rc.2", "7.78.0-rc.2-1"},
		{"dash RC", "", "78.0-rc.2", "7.78.0-rc.2-1"},
		{"explicit major bare minor", "7", "78", "7.78"},
		{"explicit major + patch", "7", "78.0", "7.78.0-1"},
		{"custom beta tag", "", "78.0-beta-extensions", "7.78.0-beta-extensions-1"},
		{"custom beta tag idempotent", "", "78.0-beta-extensions-1", "7.78.0-beta-extensions-1"},
		{"long custom tag", "", "78.0-beta-byoc-integration-test", "7.78.0-beta-byoc-integration-test-1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("DD_AGENT_MAJOR_VERSION", c.major)
			t.Setenv("DD_AGENT_MINOR_VERSION", c.minor)
			got := resolveAgentOCITag(env.FromEnv())
			if got != c.want {
				t.Errorf("resolveAgentOCITag(MAJOR=%q MINOR=%q) = %q, want %q",
					c.major, c.minor, got, c.want)
			}
		})
	}
}
