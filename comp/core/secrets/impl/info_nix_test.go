// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package secretsimpl

import (
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
)

var (
	testConfInfo = []byte(`---
instances:
- password: ENC[pass3]
- password: ENC[pass2]
`)
)

func TestGetExecutablePermissionsError(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	_, err := resolver.getExecutablePermissions()
	assert.Error(t, err, "getExecutablePermissions should fail when secretBackendCommand file does not exists")
}

func setupSecretCommand(t *testing.T, resolver *secretResolver) (string, string) {
	dir := t.TempDir()

	resolver.backendCommand = filepath.Join(dir, "executable")
	f, err := os.Create(resolver.backendCommand)
	require.NoError(t, err)
	f.Close()
	os.Chmod(resolver.backendCommand, 0700)

	u, err := user.Current()
	require.NoError(t, err)
	currentUser, err := user.LookupId(u.Uid)
	require.NoError(t, err)
	currentGroup, err := user.LookupGroupId(u.Gid)
	require.NoError(t, err)

	return currentUser.Username, currentGroup.Name
}

func TestGetExecutablePermissionsSuccess(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	currentUser, currentGroup := setupSecretCommand(t, resolver)

	res, err := resolver.getExecutablePermissions()
	require.NoError(t, err)
	assert.Equal(t, "100700", res.FileMode)
	assert.Equal(t, currentUser, res.Owner)
	assert.Equal(t, currentGroup, res.Group)
}

func TestDebugInfo(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	currentUser, currentGroup := setupSecretCommand(t, resolver)

	resolver.commandHookFunc = func(string) ([]byte, error) {
		res := []byte("{\"pass1\":{\"value\":\"password1\"},")
		res = append(res, []byte("\"pass2\":{\"value\":\"password2\"},")...)
		res = append(res, []byte("\"pass3\":{\"value\":\"password3\"}}")...)
		return res, nil
	}

	_, err := resolver.Resolve(testConf, "test", "", "", true)
	require.NoError(t, err)
	_, err = resolver.Resolve(testConfInfo, "test2", "", "", true)
	require.NoError(t, err)

	debugInfo := make(map[string]interface{})
	resolver.getDebugInfo(debugInfo, false)

	assert.True(t, debugInfo["backendCommandSet"].(bool))
	assert.Equal(t, resolver.backendCommand, debugInfo["executable"].(string))
	assert.Equal(t, "OK, the executable has the correct permissions", debugInfo["executablePermissions"].(string))
	assert.True(t, debugInfo["executablePermissionsOK"].(bool))
	assert.False(t, debugInfo["refreshIntervalEnabled"].(bool))

	handles := debugInfo["handles"].(map[string][][]string)
	assert.Len(t, handles, 3)

	assert.Contains(t, handles, "pass1")
	assert.Contains(t, handles, "pass2")
	assert.Contains(t, handles, "pass3")

	assert.Len(t, handles["pass1"], 1)
	assert.Equal(t, []string{"test", "instances/0/password"}, handles["pass1"][0])

	assert.Len(t, handles["pass2"], 2)
	expectedPass2 := [][]string{
		{"test", "instances/1/password"},
		{"test2", "instances/1/password"},
	}
	assert.ElementsMatch(t, expectedPass2, handles["pass2"])

	assert.Len(t, handles["pass3"], 1)
	assert.Equal(t, []string{"test2", "instances/0/password"}, handles["pass3"][0])

	var buffer bytes.Buffer
	err = resolver.Text(false, &buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "Executable path: "+resolver.backendCommand)
	assert.Contains(t, output, "OK, the executable has the correct permissions")
	assert.Contains(t, output, "Owner: "+currentUser)
	assert.Contains(t, output, "Group: "+currentGroup)
	assert.Contains(t, output, "Number of secrets resolved: 3")
	assert.Contains(t, output, "'secret_refresh_interval' is disabled")
}

func TestDebugInfoError(t *testing.T) {
	tel := nooptelemetry.GetCompatComponent()
	resolver := newEnabledSecretResolver(tel)
	resolver.backendCommand = "some_command"

	resolver.commandHookFunc = func(string) ([]byte, error) {
		res := []byte("{\"pass1\":{\"value\":\"password1\"},")
		res = append(res, []byte("\"pass2\":{\"value\":\"password2\"},")...)
		res = append(res, []byte("\"pass3\":{\"value\":\"password3\"}}")...)
		return res, nil
	}

	_, err := resolver.Resolve(testConf, "test", "", "", true)
	require.NoError(t, err)
	_, err = resolver.Resolve(testConfInfo, "test2", "", "", true)
	require.NoError(t, err)

	debugInfo := make(map[string]interface{})
	resolver.getDebugInfo(debugInfo, false)

	assert.True(t, debugInfo["backendCommandSet"].(bool))
	assert.Equal(t, "some_command", debugInfo["executable"].(string))
	assert.Equal(t, "error: the executable does not have the correct permissions", debugInfo["executablePermissions"].(string))
	assert.False(t, debugInfo["executablePermissionsOK"].(bool))
	assert.Contains(t, debugInfo["executablePermissionsDetailsError"].(string), "no such file or directory")
	assert.False(t, debugInfo["refreshIntervalEnabled"].(bool))

	handles := debugInfo["handles"].(map[string][][]string)
	assert.Len(t, handles, 3)

	var buffer bytes.Buffer
	err = resolver.Text(false, &buffer)
	require.NoError(t, err)

	output := buffer.String()
	assert.Contains(t, output, "Executable path: some_command")
	assert.Contains(t, output, "error: the executable does not have the correct permissions")
	assert.Contains(t, output, "no such file or directory")
	assert.Contains(t, output, "Number of secrets resolved: 3")
	assert.Contains(t, output, "'secret_refresh_interval' is disabled")
}
