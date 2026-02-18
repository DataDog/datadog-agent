// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsimpl

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

func TestSecretStatusOutput(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.Configure(secrets.ConfigParams{
		Command:       "/path/to/command",
		GroupExecPerm: false,
	})

	stats := make(map[string]interface{})
	err := resolver.JSON(false, stats)
	require.NoError(t, err)
	assert.NotEmpty(t, stats)

	b := new(bytes.Buffer)
	err = resolver.Text(false, b)
	require.NoError(t, err)
	assert.NotEmpty(t, b.String())

	b = new(bytes.Buffer)
	err = resolver.HTML(false, b)
	require.NoError(t, err)
	assert.NotEmpty(t, b.String())
}

func TestSecretStatusWithNoBackendCommand(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.Configure(secrets.ConfigParams{})

	stats := make(map[string]interface{})
	err := resolver.JSON(false, stats)
	require.NoError(t, err)
	require.Contains(t, stats, "message")
	require.Equal(t, false, stats["backendCommandSet"])
	require.Equal(t, "No secret_backend_command set: secrets feature is not enabled", stats["message"])
}

func TestSecretStatusHandles(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.Configure(secrets.ConfigParams{
		Command:       "/path/to/command",
		GroupExecPerm: false,
	})
	resolver.origin = map[string][]secretContext{
		"handle1": {
			{
				origin: "config.yaml",
				path:   []string{"path", "to", "secret"},
			},
		},
		"handle2": {
			{
				origin: "another_config.yaml",
				path:   []string{"another", "path"},
			},
			{
				origin: "third_config.yaml",
				path:   []string{"third", "path"},
			},
		},
	}

	stats := make(map[string]interface{})
	err := resolver.JSON(false, stats)
	require.NoError(t, err)

	require.Contains(t, stats, "handles")
	handles, ok := stats["handles"].(map[string][][]string)
	assert.True(t, ok)

	assert.Equal(t, handles, map[string][][]string{
		"handle1": {{"config.yaml", "path/to/secret"}},
		"handle2": {
			{"another_config.yaml", "another/path"},
			{"third_config.yaml", "third/path"},
		},
	})
}

func TestSecretStatusWithPermissions(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.Configure(secrets.ConfigParams{
		Command: "/path/to/command",
		//GroupExecPerm: false,
	})

	defer func() { checkRightsFunc = filesystem.CheckRights }()

	checkRightsFunc = func(_ string, _ bool) error { return nil }

	stats := make(map[string]interface{})
	err := resolver.JSON(false, stats)
	require.NoError(t, err)
	require.Contains(t, stats, "executablePermissionsOK")
	assert.Equal(t, true, stats["backendCommandSet"])
	assert.Equal(t, true, stats["executablePermissionsOK"])
	require.Contains(t, stats, "executablePermissions")
	assert.Equal(t, "OK, the executable has the correct permissions", stats["executablePermissions"])

	checkRightsFunc = func(_ string, _ bool) error { return errors.New("some error") }

	stats = make(map[string]interface{})
	err = resolver.JSON(false, stats)
	require.NoError(t, err)
	require.Contains(t, stats, "executablePermissionsOK")
	assert.Equal(t, true, stats["backendCommandSet"])
	assert.Equal(t, false, stats["executablePermissionsOK"])
	require.Contains(t, stats, "executablePermissions")
	assert.Equal(t, "error: the executable does not have the correct permissions", stats["executablePermissions"])
	assert.Equal(t, "some error", stats["executablePermissionsError"])
}

func TestSecretStatusNative(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.Configure(secrets.ConfigParams{
		Type: "aws.secrets",
	})

	stats := make(map[string]interface{})
	err := resolver.JSON(false, stats)
	require.NoError(t, err)
	require.Contains(t, stats, "executablePermissionsOK")
	assert.Equal(t, true, stats["backendCommandSet"])
	assert.Equal(t, true, stats["executablePermissionsOK"])
	require.Contains(t, stats, "executablePermissions")
	assert.Equal(t, "OK, native secret generic connector used", stats["executablePermissions"])
}

func TestSecretRefreshInterval(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.Configure(secrets.ConfigParams{
		Type: "aws.secrets",
	})

	stats := make(map[string]interface{})
	err := resolver.JSON(false, stats)
	require.NoError(t, err)
	require.False(t, stats["refreshIntervalEnabled"].(bool))

	resolver.Configure(secrets.ConfigParams{
		Type:            "aws.secrets",
		RefreshInterval: 15,
	})

	stats = make(map[string]interface{})
	err = resolver.JSON(false, stats)
	require.NoError(t, err)
	require.True(t, stats["refreshIntervalEnabled"].(bool))
	require.Equal(t, "15s", stats["refreshInterval"])
}
