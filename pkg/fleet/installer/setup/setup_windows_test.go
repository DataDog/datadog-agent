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

func TestApplyAgentPipelineID(t *testing.T) {
	cases := []struct {
		name             string
		pipelineID       string
		channel          string
		major            string
		minor            string
		existingRegistry string
		existingVersion  string
		wantRegistry     string
		wantVersionTag   string
		wantErr          bool
	}{
		{name: "unset is a no-op", pipelineID: ""},
		{name: "sets registry and version tag", pipelineID: "118008542", wantRegistry: pipelineRegistry, wantVersionTag: "pipeline-118008542"},
		{name: "conflict with beta channel", pipelineID: "118008542", channel: channelBeta, wantErr: true},
		{name: "conflict with stable channel", pipelineID: "118008542", channel: channelStable, wantErr: true},
		{name: "conflict with minor version", pipelineID: "118008542", minor: "79.0", wantErr: true},
		{name: "conflict with major version", pipelineID: "118008542", major: "7", wantErr: true},
		// An explicit low-level override wins for its axis; the pipeline
		// default only fills in the axis the user did not set.
		{name: "explicit registry override wins", pipelineID: "118008542", existingRegistry: "user.registry.example.com", wantRegistry: "user.registry.example.com", wantVersionTag: "pipeline-118008542"},
		{name: "explicit version override wins", pipelineID: "118008542", existingVersion: "7.79.0-1", wantRegistry: pipelineRegistry, wantVersionTag: "7.79.0-1"},
		// A child re-exec inherits the parent's overrides via FromEnv, so it
		// sees the values already set and keeps them.
		{name: "propagated overrides are kept", pipelineID: "118008542", existingRegistry: pipelineRegistry, existingVersion: "pipeline-118008542", wantRegistry: pipelineRegistry, wantVersionTag: "pipeline-118008542"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Model what FromEnv produces: an "existing" override is present
			// in both the env var and the map.
			t.Setenv(envInstallerRegistryURLAgent, c.existingRegistry)
			t.Setenv(envInstallerDefaultVersionAgent, c.existingVersion)
			e := &env.Env{
				AgentPipelineID:                c.pipelineID,
				AgentDistChannel:               c.channel,
				AgentMajorVersion:              c.major,
				AgentMinorVersion:              c.minor,
				RegistryOverrideByImage:        map[string]string{},
				DefaultPackagesVersionOverride: map[string]string{},
			}
			if c.existingRegistry != "" {
				e.RegistryOverrideByImage[agentPackageImage] = c.existingRegistry
			}
			if c.existingVersion != "" {
				e.DefaultPackagesVersionOverride[agentPackage] = c.existingVersion
			}
			err := applyAgentPipelineID(e)
			if c.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, c.wantRegistry, e.RegistryOverrideByImage[agentPackageImage])
			assert.Equal(t, c.wantVersionTag, e.DefaultPackagesVersionOverride[agentPackage])
			assert.Equal(t, c.wantRegistry, os.Getenv(envInstallerRegistryURLAgent))
			assert.Equal(t, c.wantVersionTag, os.Getenv(envInstallerDefaultVersionAgent))
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
