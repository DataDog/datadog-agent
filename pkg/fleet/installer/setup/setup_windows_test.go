// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package setup

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

func TestRequestedAgentVersion(t *testing.T) {
	cases := []struct {
		name     string
		major    string
		minor    string
		override string // DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT
		want     string
		wantErr  bool
	}{
		// "unset" returns "" (no error) — caller falls through to in-process setup.
		{name: "unset returns empty string", want: ""},

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
		{name: "override=latest hands off to the latest tag", override: "latest", want: "latest"},

		// Major must be empty or "7" — anything else is a user error.
		{name: "MAJOR=latest is an error", major: "latest", wantErr: true},
		{name: "MAJOR=latest with MINOR is an error (no garbage tag)", major: "latest", minor: "79", wantErr: true},
		{name: "MAJOR=6 is an error (unsupported on Windows fleet)", major: "6", wantErr: true},
		{name: "MAJOR=8 is an error (future major not yet supported)", major: "8", wantErr: true},

		// Minor floor: Windows handoff requires Agent 7.72+ (the version
		// where datadog-installer.exe was publicly released with the
		// `setup --flavor default` flow).
		{name: "MINOR=72 is at the floor", minor: "72", want: "7.72"},
		{name: "MINOR=72.0 is at the floor", minor: "72.0", want: "7.72.0-1"},
		{name: "MINOR=71 is below the floor", minor: "71", wantErr: true},
		{name: "MINOR=65 is below the floor (matches bootstrap fleet-automation minimum)", minor: "65.0", wantErr: true},
		{name: "MINOR=latest is rejected (non-numeric)", minor: "latest", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("DD_AGENT_MAJOR_VERSION", c.major)
			t.Setenv("DD_AGENT_MINOR_VERSION", c.minor)
			t.Setenv("DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_AGENT", c.override)
			got, err := requestedAgentVersion(env.FromEnv())
			if c.wantErr {
				if err == nil {
					t.Fatalf("requestedAgentVersion(MAJOR=%q MINOR=%q OVERRIDE=%q) = %q, nil; want error",
						c.major, c.minor, c.override, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("requestedAgentVersion(MAJOR=%q MINOR=%q OVERRIDE=%q) returned unexpected error: %v",
					c.major, c.minor, c.override, err)
			}
			if got != c.want {
				t.Errorf("requestedAgentVersion(MAJOR=%q MINOR=%q OVERRIDE=%q) = %q, want %q",
					c.major, c.minor, c.override, got, c.want)
			}
		})
	}
}

func TestApplyAgentDistChannel(t *testing.T) {
	cases := []struct {
		name             string
		channel          string
		existingOverride string
		wantOverride     string
		wantEnvVar       string
		wantErr          bool
	}{
		{name: "unset is a no-op", channel: ""},
		{name: "stable is a no-op", channel: channelStable},
		{name: "beta sets per-image override and env var", channel: channelBeta, wantOverride: betaRegistry, wantEnvVar: betaRegistry},
		{name: "user-provided override wins over beta", channel: channelBeta, existingOverride: "user.registry.example.com", wantOverride: "user.registry.example.com", wantEnvVar: "user.registry.example.com"},
		{name: "bad-value returns error", channel: "bad-value", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(envInstallerRegistryURLAgent, c.existingOverride)
			e := &env.Env{AgentDistChannel: c.channel, RegistryOverrideByImage: map[string]string{}}
			if c.existingOverride != "" {
				e.RegistryOverrideByImage[agentPackageImage] = c.existingOverride
			}
			err := applyAgentDistChannel(e)
			if c.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, c.wantOverride, e.RegistryOverrideByImage[agentPackageImage])
			assert.Equal(t, c.wantEnvVar, os.Getenv(envInstallerRegistryURLAgent))
		})
	}
}
