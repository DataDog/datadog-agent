// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package configbackupimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// testLifecycle is a no-op lifecycle for use in tests.
type testLifecycle struct{}

func (testLifecycle) Append(compdef.Hook) {}

// makeDeps builds the component dependencies with mock config, sysprobe
// config and log components.
func makeDeps(t *testing.T) Requires {
	t.Helper()
	return Requires{
		Config:         config.NewMock(t),
		SysprobeConfig: sysprobeconfigmock.NewMock(t),
		Log:            logmock.New(t),
		Lc:             testLifecycle{},
	}
}

// makeBackup builds a *configBackup wired to the mock deps.
func makeBackup(t *testing.T) *configBackup {
	t.Helper()
	deps := makeDeps(t)
	return &configBackup{
		config:         deps.Config,
		sysprobeConfig: deps.SysprobeConfig,
		log:            deps.Log,
	}
}

// writeConfigDir creates a temporary config directory with a datadog.yaml
// and returns the directory and a mock config whose ConfigFileUsed points at
// it.
func writeConfigDir(t *testing.T) (*configBackup, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("api_key: 0000001\n"), 0o600))
	cb := makeBackup(t)
	cb.config = config.NewMockFromYAMLFile(t, filepath.Join(dir, "datadog.yaml"))
	return cb, dir
}
