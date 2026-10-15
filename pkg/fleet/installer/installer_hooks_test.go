// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

type observingHooks struct {
	packages.Hooks
	preInstall        func(string) error
	preStopExperiment func() error
}

func (h observingHooks) PreInstall(ctx context.Context, pkg string, pkgType packages.PackageType, upgrade bool, path string) error {
	if h.preInstall != nil {
		return h.preInstall(path)
	}
	return h.Hooks.PreInstall(ctx, pkg, pkgType, upgrade, path)
}

func (h observingHooks) PreStopExperiment(ctx context.Context, pkg string) error {
	if h.preStopExperiment != nil {
		return h.preStopExperiment()
	}
	return h.Hooks.PreStopExperiment(ctx, pkg)
}

func TestPreInstallIncomingPayloadLifetime(t *testing.T) {
	s := fixtures.NewServer(t)
	i := newTestPackageManager(t, s, t.TempDir())
	t.Cleanup(func() { require.NoError(t, i.db.Close()) })
	i.testHooks.noop = true
	var stagingPath string
	i.hooks = observingHooks{Hooks: i.testHooks, preInstall: func(path string) error {
		stagingPath = path
		fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), os.DirFS(path))
		assert.NoDirExists(t, i.packages.Get("simple").StablePath())
		return nil
	}}
	require.NoError(t, i.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil))
	assert.NoDirExists(t, stagingPath, "staging directory has been moved to the repository")

	failed := errors.New("pre-install failure")
	i.hooks = observingHooks{Hooks: i.testHooks, preInstall: func(path string) error {
		stagingPath = path
		fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), os.DirFS(path))
		fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), i.packages.Get("simple").StableFS())
		return failed
	}}
	require.ErrorIs(t, i.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV2), nil), failed)
	assert.NoDirExists(t, stagingPath, "a failed pre-install must clean its staging directory")
	installed, err := i.db.GetPackage("simple")
	require.NoError(t, err)
	assert.Equal(t, "v1", installed.Version)
	state, err := i.packages.Get("simple").GetState()
	require.NoError(t, err)
	assert.Equal(t, "v1", state.Stable)
}

func TestAgentExperimentHookPayloadLifetime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("delete-before-hook is a Unix Agent/installer compatibility path")
	}
	for _, pkg := range []string{packageDatadogAgent, packageDatadogInstaller} {
		for _, layout := range []string{"legacy", "package-owned", "invalid"} {
			t.Run(pkg+"/"+layout, func(t *testing.T) {
				repos := repository.NewRepositories(t.TempDir(), nil)
				stable := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(stable, "version"), []byte("v1"), 0644))
				require.NoError(t, repos.Create(testCtx, pkg, "v1", stable))
				experiment := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(experiment, "version"), []byte("v2"), 0644))
				switch layout {
				case "package-owned":
					require.NoError(t, os.Mkdir(filepath.Join(experiment, "hooks"), 0755))
				case "invalid":
					require.NoError(t, os.WriteFile(filepath.Join(experiment, "hooks"), nil, 0644))
				}
				repo := repos.Get(pkg)
				require.NoError(t, repo.SetExperiment(testCtx, "v2", experiment))
				called := false
				i := &installerImpl{packages: repos, hooks: observingHooks{
					Hooks: &testHooks{noop: true},
					preStopExperiment: func() error {
						called = true
						version, err := os.ReadFile(filepath.Join(repo.ExperimentPath(), "version"))
						require.NoError(t, err)
						if layout == "package-owned" {
							assert.Equal(t, "v2", string(version))
						} else {
							assert.Equal(t, "v1", string(version), "legacy hook must still run after experiment deletion")
						}
						return nil
					},
				}}
				err := i.RemoveExperiment(testCtx, pkg)
				if layout == "invalid" {
					require.Error(t, err)
					assert.False(t, called)
					state, err := repo.GetState()
					require.NoError(t, err)
					assert.Equal(t, "v2", state.Experiment)
				} else {
					require.NoError(t, err)
					assert.True(t, called)
					state, err := repo.GetState()
					require.NoError(t, err)
					assert.False(t, state.HasExperiment())
				}
			})
		}
	}
}
