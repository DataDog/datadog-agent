// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package installer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
)

// The three tests below exercise the experiment lifecycle through the OCI flow, from the fixture
// registry. They are tagged !darwin rather than left untagged because on macOS installExperiment
// is a different implementation reading a different kind of artifact: the experiment payload is a
// per-version .pkg that only installer(8) may unpack, so there is no OCI image to extract and the
// fixture server cannot serve one it would accept. The macOS half of the same lifecycle is
// covered by installer_darwin_test.go for the flow, and by
// packages/version_experiment_darwin_test.go for what the hooks do around it.

func TestInstallExperiment(t *testing.T) {
	doTestInstallers(t, func(instFactory installFnFactory, t *testing.T) {
		s := fixtures.NewServer(t)
		installer := newTestPackageManager(t, s, t.TempDir())
		defer installer.db.Close()

		preInstallCall := installer.testHooks.On("PreInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false).Return(nil)
		installer.testHooks.On("PostInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false, mock.Anything).Return(nil).NotBefore(preInstallCall)
		err := instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)
		preStartExperimentCall := installer.testHooks.On("PreStartExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil)
		installer.testHooks.On("PostStartExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil).NotBefore(preStartExperimentCall)
		err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
		assert.NoError(t, err)
		r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
		state, err := r.GetState()
		assert.NoError(t, err)
		assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
		assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Experiment)
		fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
		fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.ExperimentFS())
	})
}

func TestInstallPromoteExperiment(t *testing.T) {
	doTestInstallers(t, func(instFactory installFnFactory, t *testing.T) {
		s := fixtures.NewServer(t)
		installer := newTestPackageManager(t, s, t.TempDir())
		defer installer.db.Close()

		preInstallCall := installer.testHooks.On("PreInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false).Return(nil)
		installer.testHooks.On("PostInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false, mock.Anything).Return(nil).NotBefore(preInstallCall)
		err := instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)
		preStartExperimentCall := installer.testHooks.On("PreStartExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil)
		installer.testHooks.On("PostStartExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil).NotBefore(preStartExperimentCall)
		err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
		assert.NoError(t, err)
		prePromoteExperimentCall := installer.testHooks.On("PrePromoteExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil)
		installer.testHooks.On("PostPromoteExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil).NotBefore(prePromoteExperimentCall)
		err = installer.PromoteExperiment(testCtx, fixtures.FixtureSimpleV1.Package)
		assert.NoError(t, err)
		r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
		state, err := r.GetState()
		assert.NoError(t, err)
		assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Stable)
		assert.False(t, state.HasExperiment())
		fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.StableFS())
	})
}

func TestUninstallExperiment(t *testing.T) {
	doTestInstallers(t, func(instFactory installFnFactory, t *testing.T) {
		s := fixtures.NewServer(t)
		installer := newTestPackageManager(t, s, t.TempDir())
		defer installer.db.Close()

		preInstallCall := installer.testHooks.On("PreInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false).Return(nil)
		installer.testHooks.On("PostInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false, mock.Anything).Return(nil).NotBefore(preInstallCall)
		err := instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)
		preStartExperimentCall := installer.testHooks.On("PreStartExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil)
		installer.testHooks.On("PostStartExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil).NotBefore(preStartExperimentCall)
		err = installer.InstallExperiment(testCtx, s.PackageURL(fixtures.FixtureSimpleV2))
		assert.NoError(t, err)
		preStopExperimentCall := installer.testHooks.On("PreStopExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil)
		installer.testHooks.On("PostStopExperiment", testCtx, fixtures.FixtureSimpleV1.Package).Return(nil).NotBefore(preStopExperimentCall)
		err = installer.RemoveExperiment(testCtx, fixtures.FixtureSimpleV1.Package)
		assert.NoError(t, err)
		r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
		state, err := r.GetState()
		assert.NoError(t, err)
		assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
		assert.False(t, state.HasExperiment())
		fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	})
}
