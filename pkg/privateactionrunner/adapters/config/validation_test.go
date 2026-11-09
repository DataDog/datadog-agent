// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestValidateWarnsForRCWithoutPARURL(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	cfg := configmock.New(t)
	cfg.SetInTest(par.Enabled, true)
	cfg.SetInTest("site", "datad0g.com")
	for _, rcURL := range []string{"https://config.datad0g.com.", "https://config.datadoghq.com."} {
		cfg.SetInTest("remote_configuration.rc_dd_url", rcURL)
		warnings := captureValidationWarnings(t, func() {
			err := Validate(cfg)
			require.NoError(t, err)
		})
		require.Contains(t, warnings, "[WARN]")
		require.Contains(t, warnings, "the RC override does not change PAR routing")
		require.Contains(t, warnings, "Set private_action_runner.par_dd_url explicitly")
		require.NotContains(t, warnings, "nonstandard")
	}

	cfg.SetInTest(par.RunnerURL, "https://par-proxy.example.com/prefix")
	warnings := captureValidationWarnings(t, func() {
		err := Validate(cfg)
		require.NoError(t, err)
	})
	require.NotContains(t, warnings, "remote_configuration.rc_dd_url")
}

func TestValidatePARRestrictedShellPrivilegedElevatableCommandsWarnsForUnnamespaced(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(par.PrivateKey, "")
	mockConfig.SetInTest(par.URN, "")
	mockConfig.SetInTest(par.RestrictedShellPrivilegedElevatableCommands, []string{"truncate", "rshell:journalctl"})

	logs := captureValidationWarnings(t, func() {
		err := Validate(mockConfig)
		require.NoError(t, err)
	})

	assert.Contains(t, logs, par.RestrictedShellPrivilegedElevatableCommands)
	assert.Contains(t, logs, `"truncate"`)
	assert.Contains(t, logs, `"rshell:"`)
	assert.Contains(t, logs, `"rshell:truncate"`)
	assert.NotContains(t, logs, `"rshell:journalctl"`)
}

func TestValidatePARRestrictedShellAllowedPathsWarnsForBackslash(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(par.PrivateKey, "")
	mockConfig.SetInTest(par.URN, "")
	mockConfig.SetInTest(par.RestrictedShellAllowedPaths, []string{`C:\Data`, "/var/log"})

	logs := captureValidationWarnings(t, func() {
		err := Validate(mockConfig)
		require.NoError(t, err)
	})

	assert.Contains(t, logs, par.RestrictedShellAllowedPaths)
	assert.Contains(t, logs, `C:\\Data`)
	assert.Contains(t, logs, "contains a backslash")
	assert.Contains(t, logs, "only forward-slash paths are supported")
	assert.NotContains(t, logs, "/var/log")
}

func TestValidatePARRestrictedShellAllowedPathsWarnsForNonDirectory(t *testing.T) {
	tmpDir := filepath.ToSlash(t.TempDir())
	fp := filepath.ToSlash(filepath.Join(tmpDir, "file.txt"))
	require.NoError(t, os.WriteFile(fp, []byte("x"), 0o600))

	mockConfig := configmock.New(t)
	mockConfig.SetInTest(par.PrivateKey, "")
	mockConfig.SetInTest(par.URN, "")
	mockConfig.SetInTest(par.RestrictedShellAllowedPaths, []string{tmpDir, fp})

	logs := captureValidationWarnings(t, func() {
		err := Validate(mockConfig)
		require.NoError(t, err)
	})

	assert.Contains(t, logs, par.RestrictedShellAllowedPaths)
	assert.Contains(t, logs, fp)
	assert.Contains(t, logs, "is not a directory")
	assert.Contains(t, logs, "Use the containing directory instead")
	assert.NotContains(t, logs, `entry "`+tmpDir+`" is not a directory`)
}

func TestValidatePARRestrictedShellAllowedCommandsWarnsForUnnamespaced(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(par.PrivateKey, "")
	mockConfig.SetInTest(par.URN, "")
	mockConfig.SetInTest(par.RestrictedShellAllowedCommands, []string{"cat", "rshell:ls"})

	logs := captureValidationWarnings(t, func() {
		err := Validate(mockConfig)
		require.NoError(t, err)
	})

	assert.Contains(t, logs, par.RestrictedShellAllowedCommands)
	assert.Contains(t, logs, `"cat"`)
	assert.Contains(t, logs, `"rshell:"`)
	assert.Contains(t, logs, `"rshell:cat"`)
	assert.NotContains(t, logs, `"rshell:ls"`)
}

func TestValidatePARRestrictedShellAllowedCommandsDefaultDoesNotWarn(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest(par.PrivateKey, "")
	mockConfig.SetInTest(par.URN, "")

	logs := captureValidationWarnings(t, func() {
		err := Validate(mockConfig)
		require.NoError(t, err)
	})

	assert.Empty(t, logs)
}

func captureValidationWarnings(t *testing.T, fn func()) string {
	t.Helper()

	var logBuffer bytes.Buffer
	logWriter := bufio.NewWriter(&logBuffer)
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(logWriter, log.WarnLvl)
	require.NoError(t, err)

	previousLogger := log.Default()
	t.Cleanup(func() {
		log.SetupLogger(previousLogger, "debug")
	})
	log.SetupLogger(logger, "warn")

	fn()

	require.NoError(t, logWriter.Flush())
	return logBuffer.String()
}
