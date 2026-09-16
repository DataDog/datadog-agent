// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installscript

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	compfakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

func TestSplitAgentVersion(t *testing.T) {
	cases := []struct {
		name      string
		version   string
		wantMajor string
		wantMinor string
		wantOK    bool
	}{
		{"latest is unparsed", "latest", "", "", false},
		{"empty is unparsed", "", "", "", false},
		{"major 7 with minor", "7.65.0", "7", "65.0", true},
		{"major 6 with minor", "6.53.0", "6", "53.0", true},
		{"bare major", "7.", "7", "", true},
		{"unsupported major", "5.1.0", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			major, minor, ok := splitAgentVersion(tc.version)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantMajor, major)
			assert.Equal(t, tc.wantMinor, minor)
		})
	}
}

func TestBuildAgentConfig(t *testing.T) {
	env := &environments.Host{FakeIntake: &components.FakeIntake{
		FakeintakeOutput: compfakeintake.FakeintakeOutput{
			Scheme: "http",
			Host:   "fakeintake.internal",
			Port:   8080,
		},
	}}

	config, err := buildAgentConfig(env, "test-api-key", "logs_enabled: true")
	require.NoError(t, err)
	assert.Contains(t, config, `api_key: test-api-key`)
	assert.Contains(t, config, `dd_url: http://fakeintake.internal:8080`)
	assert.Contains(t, config, `logs_dd_url: fakeintake.internal:8080`)
	assert.Contains(t, config, `logs_enabled: true`)
}

func TestCommand(t *testing.T) {
	t.Run("pinned version sets major and minor", func(t *testing.T) {
		cmd := command("7.65.0", "test-api-key")
		assert.Contains(t, cmd, "DD_API_KEY=test-api-key")
		assert.Contains(t, cmd, "DD_AGENT_MAJOR_VERSION=7")
		assert.Contains(t, cmd, "DD_AGENT_MINOR_VERSION=65.0")
		assert.Contains(t, cmd, "install_script_agent7.sh")
	})

	t.Run("latest omits version env vars", func(t *testing.T) {
		cmd := command("latest", "test-api-key")
		assert.Contains(t, cmd, "DD_API_KEY=test-api-key")
		assert.NotContains(t, cmd, "DD_AGENT_MAJOR_VERSION")
		assert.NotContains(t, cmd, "DD_AGENT_MINOR_VERSION")
		assert.Contains(t, cmd, "install_script_agent7.sh", "still defaults to the major-7 script")
	})
}
