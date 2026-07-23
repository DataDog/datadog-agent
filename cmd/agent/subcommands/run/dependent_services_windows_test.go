// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestServicedefIsEnabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("process_config.enabled", true, model.SourceDefault)

	svc := Servicedef{
		name: "process",
		configKeys: map[string]model.Reader{
			"process_config.enabled": cfg,
		},
	}

	t.Run("enabled by config", func(t *testing.T) {
		assert.True(t, svc.IsEnabled(false, cfg))
		assert.True(t, svc.IsEnabled(true, cfg))
	})

	t.Run("not suppressed without definition file", func(t *testing.T) {
		cfg.Set("process_manager.enabled", true, model.SourceDefault)
		assert.True(t, svc.IsEnabled(true, cfg))
	})

	t.Run("not suppressed when process manager disabled", func(t *testing.T) {
		svc.procmgrDefinitionFile = processProcmgrDefinitionFile
		cfg.Set("process_manager.enabled", false, model.SourceDefault)
		withProcmgrInstallRoot(t, writeProcmgrDefinitionFile(t, processProcmgrDefinitionFile), func() {
			assert.True(t, svc.IsEnabled(true, cfg))
		})
	})
}

func TestServicedefIsEnabled_procmgrSuppression(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("process_config.enabled", true, model.SourceDefault)
	cfg.Set("process_manager.enabled", true, model.SourceDefault)

	svc := Servicedef{
		name:                  "process",
		procmgrDefinitionFile: processProcmgrDefinitionFile,
		configKeys: map[string]model.Reader{
			"process_config.enabled": cfg,
		},
	}

	t.Run("not suppressed when processes.d file missing", func(t *testing.T) {
		installRoot := t.TempDir()
		withProcmgrInstallRoot(t, installRoot, func() {
			assert.True(t, svc.IsEnabled(true, cfg))
		})
	})

	t.Run("suppressed when processes.d file exists and procmgr started", func(t *testing.T) {
		installRoot := writeProcmgrDefinitionFile(t, processProcmgrDefinitionFile)
		withProcmgrInstallRoot(t, installRoot, func() {
			assert.False(t, svc.IsEnabled(true, cfg))
		})
	})

	t.Run("falls back to legacy SCM when procmgr unavailable", func(t *testing.T) {
		installRoot := writeProcmgrDefinitionFile(t, processProcmgrDefinitionFile)
		withProcmgrInstallRoot(t, installRoot, func() {
			assert.True(t, svc.IsEnabled(false, cfg))
		})
	})
}

func TestServicedefIsEnabled_procmgrManagedServices(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("process_manager.enabled", true, model.SourceDefault)

	cases := []struct {
		name    string
		defFile string
		config  map[string]bool
	}{
		{
			name:    "process",
			defFile: processProcmgrDefinitionFile,
			config:  map[string]bool{"process_config.enabled": true},
		},
		{
			name:    "par",
			defFile: parProcmgrDefinitionFile,
			config:  map[string]bool{"private_action_runner.enabled": true},
		},
		{
			name:    "otel",
			defFile: ddotProcmgrDefinitionFile,
			config:  map[string]bool{"otelcollector.enabled": true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installRoot := writeProcmgrDefinitionFile(t, tc.defFile)
			withProcmgrInstallRoot(t, installRoot, func() {
				keys := make(map[string]model.Reader, len(tc.config))
				for key := range tc.config {
					keys[key] = cfg
					cfg.Set(key, tc.config[key], model.SourceDefault)
				}
				svc := Servicedef{
					name:                  tc.name,
					procmgrDefinitionFile: tc.defFile,
					configKeys:            keys,
				}
				assert.True(t, svc.IsEnabled(false, cfg))
			})
		})
	}
}

func TestServicedefShouldStop(t *testing.T) {
	assert.True(t, (&Servicedef{shouldShutdown: true}).ShouldStop())
	assert.False(t, (&Servicedef{shouldShutdown: false}).ShouldStop())
}

func TestFindService(t *testing.T) {
	svcs := []Servicedef{{name: "apm"}, {name: "procmgr"}}
	svc, ok := findService(svcs, "procmgr")
	assert.True(t, ok)
	assert.Equal(t, "procmgr", svc.name)

	_, ok = findService(svcs, "missing")
	assert.False(t, ok)
}

func TestStartProcmgrIfEnabled(t *testing.T) {
	assert.False(t, startProcmgrIfEnabled(Servicedef{}, false))
	assert.False(t, startProcmgrIfEnabled(Servicedef{name: "procmgr"}, true))
}

func withProcmgrInstallRoot(t *testing.T, installRoot string, fn func()) {
	t.Helper()
	prev := procmgrInstallRootForDefinitionCheck
	procmgrInstallRootForDefinitionCheck = func() (string, error) {
		return installRoot, nil
	}
	t.Cleanup(func() {
		procmgrInstallRootForDefinitionCheck = prev
	})
	fn()
}

func writeProcmgrDefinitionFile(t *testing.T, fileName string) string {
	t.Helper()
	installRoot := t.TempDir()
	processesDir := filepath.Join(installRoot, "processes.d")
	require.NoError(t, os.MkdirAll(processesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(processesDir, fileName), []byte("description: test\n"), 0o644))
	return installRoot
}
