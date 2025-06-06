// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"context"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

var testCtx = context.TODO()

type installFn = func(context.Context, string, []string) error
type installFnFactory = func(manager *testPackageManager) installFn

type testPackageManager struct {
	installerImpl
	testHooks *testHooks
}

func newTestPackageManager(t *testing.T, s *fixtures.Server, rootPath string) *testPackageManager {
	packages := repository.NewRepositories(rootPath, nil)
	configs := repository.NewRepositories(t.TempDir(), nil)
	db, err := db.New(filepath.Join(rootPath, "packages.db"))
	assert.NoError(t, err)
	hooks := &testHooks{}
	return &testPackageManager{
		installerImpl: installerImpl{
			env:            &env.Env{},
			db:             db,
			downloader:     oci.NewDownloader(&env.Env{}, s.Client()),
			packages:       packages,
			configs:        configs,
			userConfigsDir: t.TempDir(),
			packagesDir:    rootPath,
			hooks:          hooks,
		},
		testHooks: hooks,
	}
}

type testHooks struct {
	mock.Mock
	noop bool
}

func (h *testHooks) PreInstall(ctx context.Context, pkg string, pkgType packages.PackageType, upgrade bool) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg, pkgType, upgrade)
	return nil
}

func (h *testHooks) PostInstall(ctx context.Context, pkg string, pkgType packages.PackageType, upgrade bool, winArgs []string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg, pkgType, upgrade, winArgs)
	return nil
}

func (h *testHooks) PreRemove(ctx context.Context, pkg string, pkgType packages.PackageType, upgrade bool) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg, pkgType, upgrade)
	return nil
}

func (h *testHooks) PreStartExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (h *testHooks) PostStartExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (h *testHooks) PreStopExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (h *testHooks) PostStopExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (h *testHooks) PrePromoteExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (h *testHooks) PostPromoteExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (h *testHooks) PostStartConfigExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (h *testHooks) PreStopConfigExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (h *testHooks) PostPromoteConfigExperiment(ctx context.Context, pkg string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg)
	return nil
}

func (i *testPackageManager) ConfigFS(f fixtures.Fixture) fs.FS {
	return os.DirFS(filepath.Join(i.userConfigsDir, f.Package))
}

func TestInstallStable(t *testing.T) {
	doTestInstallers(t, func(instFactory installFnFactory, t *testing.T) {
		s := fixtures.NewServer(t)
		installer := newTestPackageManager(t, s, t.TempDir())
		defer installer.db.Close()

		preInstallCall := installer.testHooks.On("PreInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false).Return(nil)
		installer.testHooks.On("PostInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false, mock.Anything).Return(nil).NotBefore(preInstallCall)

		err := instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)
		r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
		state, err := r.GetState()
		assert.NoError(t, err)
		assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
		assert.False(t, state.HasExperiment())
		fixtures.AssertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
		fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV1), installer.ConfigFS(fixtures.FixtureSimpleV1))
	})
}

func TestInstallUpgrade(t *testing.T) {
	doTestInstallers(t, func(instFactory installFnFactory, t *testing.T) {
		s := fixtures.NewServer(t)
		installer := newTestPackageManager(t, s, t.TempDir())
		defer installer.db.Close()

		preInstallCall := installer.testHooks.On("PreInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false).Return(nil)
		installer.testHooks.On("PostInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, false, mock.Anything).Return(nil).NotBefore(preInstallCall)

		err := instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)

		preRemoveCall := installer.testHooks.On("PreRemove", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, true).Return(nil)
		preInstallCall = installer.testHooks.On("PreInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, true).Return(nil).NotBefore(preRemoveCall)
		installer.testHooks.On("PostInstall", testCtx, fixtures.FixtureSimpleV1.Package, packages.PackageTypeOCI, true, mock.Anything).Return(nil).NotBefore(preInstallCall)

		err = instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV2), nil)
		assert.NoError(t, err)
		r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
		state, err := r.GetState()
		assert.NoError(t, err)
		assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Stable)
	})
}

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
		fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
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
		fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
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
		// we do not rollback configuration examples to their previous versions currently
		fixtures.AssertEqualFS(t, s.ConfigFS(fixtures.FixtureSimpleV2), installer.ConfigFS(fixtures.FixtureSimpleV2))
	})
}

func TestInstallSkippedWhenAlreadyInstalled(t *testing.T) {
	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir())
	defer installer.db.Close()
	installer.testHooks.noop = true

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	lastModTime, err := latestModTimeFS(r.StableFS(), ".")
	assert.NoError(t, err)

	err = installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r = installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	newLastModTime, err := latestModTimeFS(r.StableFS(), ".")
	assert.NoError(t, err)
	assert.Equal(t, lastModTime, newLastModTime)
}

func TestForceInstallWhenAlreadyInstalled(t *testing.T) {
	s := fixtures.NewServer(t)
	installer := newTestPackageManager(t, s, t.TempDir())
	defer installer.db.Close()
	installer.testHooks.noop = true

	err := installer.Install(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	lastModTime, err := latestModTimeFS(r.StableFS(), ".")
	assert.NoError(t, err)

	err = installer.ForceInstall(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
	assert.NoError(t, err)
	r = installer.packages.Get(fixtures.FixtureSimpleV1.Package)
	newLastModTime, err := latestModTimeFS(r.StableFS(), ".")
	assert.NoError(t, err)
	assert.NotEqual(t, lastModTime, newLastModTime)
}

func TestReinstallAfterDBClean(t *testing.T) {
	doTestInstallers(t, func(instFactory installFnFactory, t *testing.T) {
		s := fixtures.NewServer(t)
		installer := newTestPackageManager(t, s, t.TempDir())
		defer installer.db.Close()
		installer.testHooks.noop = true
		err := instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)
		r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)
		lastModTime, err := latestModTimeFS(r.StableFS(), ".")
		assert.NoError(t, err)

		installer.db.DeletePackage(fixtures.FixtureSimpleV1.Package)

		err = instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)
		r = installer.packages.Get(fixtures.FixtureSimpleV1.Package)
		newLastModTime, err := latestModTimeFS(r.StableFS(), ".")
		assert.NoError(t, err)
		assert.NotEqual(t, lastModTime, newLastModTime)
	})
}

func latestModTimeFS(fsys fs.FS, dirPath string) (time.Time, error) {
	var latestTime time.Time

	// Read the directory entries
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return latestTime, err
	}

	for _, entry := range entries {
		// Get full path of the entry
		entryPath := path.Join(dirPath, entry.Name())

		// Get file info to access modification time
		info, err := fs.Stat(fsys, entryPath)
		if err != nil {
			return latestTime, err
		}

		// Update the latest modification time
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
		}

		// If the entry is a directory, recurse into it
		if entry.IsDir() {
			subLatestTime, err := latestModTimeFS(fsys, entryPath) // Recurse into subdirectory
			if err != nil {
				return latestTime, err
			}
			// Compare times
			if subLatestTime.After(latestTime) {
				latestTime = subLatestTime
			}
		}
	}

	return latestTime, nil
}

func TestPurge(t *testing.T) {
	doTestInstallers(t, func(instFactory installFnFactory, t *testing.T) {
		s := fixtures.NewServer(t)
		rootPath := t.TempDir()
		installer := newTestPackageManager(t, s, rootPath)
		installer.testHooks.noop = true

		err := instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)
		r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)

		state, err := r.GetState()
		assert.NoError(t, err)
		assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)

		installer.Purge(testCtx)
		assert.NoFileExists(t, filepath.Join(rootPath, "packages.db"), "purge should remove the packages database")
		assert.NoDirExists(t, rootPath, "purge should remove the packages directory")
		assert.Nil(t, installer.db, "purge should close the packages database")
	})
}

func doTestInstallers(t *testing.T, testFunc func(installFnFactory, *testing.T)) {
	t.Helper()
	installers := []installFnFactory{
		func(manager *testPackageManager) installFn {
			return manager.Install
		},
		func(manager *testPackageManager) installFn {
			return manager.ForceInstall
		},
	}
	for _, inst := range installers {
		t.Run(runtime.FuncForPC(reflect.ValueOf(inst).Pointer()).Name(), func(t *testing.T) {
			testFunc(inst, t)
		})
	}
}

func TestNoOutsideImport(t *testing.T) {
	// Root directory to start the walk
	rootDir := "."

	// Define the unwanted import path
	datadogAgentPrefix := "github.com/DataDog/datadog-agent/"
	allowedPaths := []string{
		"pkg/fleet/installer",
		"pkg/version",      // TODO: cleanup & remove
		"pkg/util/log",     // TODO: cleanup & remove
		"pkg/util/winutil", // Needed for Windows
		"pkg/template",
	}

	// Walk the directory tree
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only check .go files
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") {
			// Create a file set and parse the file
			fs := token.NewFileSet()
			node, err := parser.ParseFile(fs, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("failed to parse file: %v", err)
			}

			// Loop through the imports in the AST
			for _, imp := range node.Imports {
				// Check if the import path matches the unwanted import
				isAllowedImport := true
				if strings.HasPrefix(imp.Path.Value, "\""+datadogAgentPrefix) {
					isAllowedImport = false
					for _, allowedPath := range allowedPaths {
						if strings.HasPrefix(imp.Path.Value, "\""+datadogAgentPrefix+allowedPath) {
							isAllowedImport = true
						}
					}
				}
				if !isAllowedImport {
					t.Errorf("file %s imports %s, which is not allowed", path, imp.Path.Value)
				}
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk directory: %v", err)
	}
}

func TestWriteConfigSymlinks(t *testing.T) {
	fleetDir := t.TempDir()
	userDir := t.TempDir()
	err := os.WriteFile(filepath.Join(userDir, "datadog.yaml"), []byte("user config"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(fleetDir, "datadog.yaml"), []byte("fleet config"), 0644)
	assert.NoError(t, err)
	err = os.MkdirAll(filepath.Join(fleetDir, "conf.d"), 0755)
	assert.NoError(t, err)

	err = writeConfigSymlinks(userDir, fleetDir)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml"))
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml.override"))
	assert.FileExists(t, filepath.Join(userDir, "conf.d.override"))
	configContent, err := os.ReadFile(filepath.Join(userDir, "datadog.yaml"))
	assert.NoError(t, err)
	overrideConfigConent, err := os.ReadFile(filepath.Join(userDir, "datadog.yaml.override"))
	assert.NoError(t, err)
	assert.Equal(t, "user config", string(configContent))
	assert.Equal(t, "fleet config", string(overrideConfigConent))

	fleetDir = t.TempDir()
	err = writeConfigSymlinks(userDir, fleetDir)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml"))
	assert.NoFileExists(t, filepath.Join(userDir, "datadog.yaml.override"))
	assert.NoFileExists(t, filepath.Join(userDir, "conf.d.override"))
}

func TestConfigNames(t *testing.T) {
	// test that the config name is allowed after cleaning
	// e.g. b/c filepath.Clean on Windows will convert forward slashes to backslashes
	t.Run("allowed-after-clean", func(t *testing.T) {
		for _, f := range allowedConfigFiles {
			cleaned := cleanConfigName(f)
			assert.Equal(t, cleaned, f)
			assert.True(t, configNameAllowed(cleaned), "config name %s should be allowed", cleaned)
		}
	})
}
