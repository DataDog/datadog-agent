// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"context"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureCommand_PreservesProcessAttributes(t *testing.T) {
	cmd := exec.Command("/bin/echo", "hello")
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: "/sandbox"}

	configureCommand(cmd)

	assert.Equal(t, "/sandbox", cmd.SysProcAttr.Chroot)
	assert.True(t, cmd.SysProcAttr.Setpgid)
	assert.Zero(t, cmd.SysProcAttr.Pgid)
}

func TestNewCommand_InjectsParameters(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{
		Command:  []string{"/bin/echo"},
		Manifest: &Manifest{},
	}

	cmd, err := NewCommand(context.Background(), pkg, session, map[string]interface{}{
		"targetURL": "https://example.com",
		"count":     3,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"/bin/echo"}, cmd.Args)
	value, ok := lookupEnv(t, cmd.Env, "PAR_ENV_TARGET_URL")
	require.True(t, ok)
	assert.Equal(t, "https://example.com", value)
	value, ok = lookupEnv(t, cmd.Env, "PAR_ENV_COUNT")
	require.True(t, ok)
	assert.Equal(t, "3", value)
}

func TestNewCommand_RejectsNonObjectParameters(t *testing.T) {
	session := newTestSession(t)
	pkg := &Package{
		Command:  []string{"/bin/echo"},
		Manifest: &Manifest{},
	}

	_, err := NewCommand(context.Background(), pkg, session, []interface{}{"not", "an", "object"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object")
}

func TestNewCommand_RejectsCollisionWithAllowedEnvVar(t *testing.T) {
	session := newTestSession(t)
	t.Setenv("PAR_ENV_NAME", "preset")
	pkg := &Package{
		Command: []string{"/bin/echo"},
		Manifest: &Manifest{
			Config: ScriptConfig{AllowedEnvVars: []string{"PAR_ENV_NAME"}},
		},
	}

	_, err := NewCommand(context.Background(), pkg, session, map[string]interface{}{"name": "world"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already set")
}
