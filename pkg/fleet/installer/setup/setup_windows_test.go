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
		name     string
		major    string
		minor    string
		override string // DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT
		want     string
	}{
		{name: "unset", want: "latest"},
		{name: "bare minor", minor: "78", want: "7.78"},
		{name: "explicit patch", minor: "78.0", want: "7.78.0-1"},
		{name: "explicit patch with release suffix", minor: "78.0-1", want: "7.78.0-1"},
		{name: "tilde RC", minor: "78.0~rc.2", want: "7.78.0-rc.2-1"},
		{name: "dash RC", minor: "78.0-rc.2", want: "7.78.0-rc.2-1"},
		{name: "explicit major bare minor", major: "7", minor: "78", want: "7.78"},
		{name: "explicit major + patch", major: "7", minor: "78.0", want: "7.78.0-1"},
		{name: "major only", major: "7", want: "7"},
		{name: "custom beta tag", minor: "78.0-beta-extensions", want: "7.78.0-beta-extensions-1"},
		{name: "custom beta tag idempotent", minor: "78.0-beta-extensions-1", want: "7.78.0-beta-extensions-1"},
		{name: "long custom tag", minor: "78.0-beta-byoc-integration-test", want: "7.78.0-beta-byoc-integration-test-1"},
		// DefaultPackagesVersionOverride wins and is returned unmodified.
		{name: "override wins over user version", minor: "78.0", override: "7.79.0-rc.2-1", want: "7.79.0-rc.2-1"},
		{name: "override wins when user version unset", override: "7.79.0-rc.2-1", want: "7.79.0-rc.2-1"},
		{name: "override returned unmodified with tilde", override: "7.79.0~rc.2", want: "7.79.0~rc.2"},
		{name: "override returned unmodified without release suffix", override: "7.79.0", want: "7.79.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("DD_AGENT_MAJOR_VERSION", c.major)
			t.Setenv("DD_AGENT_MINOR_VERSION", c.minor)
			t.Setenv("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT", c.override)
			got := resolveAgentOCITag(env.FromEnv())
			if got != c.want {
				t.Errorf("resolveAgentOCITag(MAJOR=%q MINOR=%q OVERRIDE=%q) = %q, want %q",
					c.major, c.minor, c.override, got, c.want)
			}
		})
	}
}
