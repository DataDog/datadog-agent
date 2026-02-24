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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/db"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	extensionsPkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
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
	extensionsPkg.ExtensionsDBDir = filepath.Join(rootPath, "run")
	os.MkdirAll(extensionsPkg.ExtensionsDBDir, 0755)
	packages := repository.NewRepositories(rootPath, nil)
	err := os.MkdirAll(filepath.Join(rootPath, "run"), 0755)
	assert.NoError(t, err)
	db, err := db.New(filepath.Join(rootPath, "packages.db"))
	assert.NoError(t, err)
	hooks := &testHooks{}
	userConfigsDir := t.TempDir()
	config := &config.Directories{
		StablePath:     userConfigsDir,
		ExperimentPath: t.TempDir(),
	}
	return &testPackageManager{
		installerImpl: installerImpl{
			env:            &env.Env{},
			db:             db,
			downloader:     oci.NewDownloader(&env.Env{}, s.Client()),
			packages:       packages,
			userConfigsDir: userConfigsDir,
			config:         config,
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

func (h *testHooks) PreInstallExtension(ctx context.Context, pkg string, extension string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg, extension)
	return nil
}

func (h *testHooks) PreRemoveExtension(ctx context.Context, pkg string, extension string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg, extension)
	return nil
}

func (h *testHooks) PostInstallExtension(ctx context.Context, pkg string, extension string) error {
	if h.noop {
		return nil
	}
	h.Called(ctx, pkg, extension)
	return nil
}

func (i *testPackageManager) ConfigFS(_ fixtures.Fixture) fs.FS {
	return os.DirFS(filepath.Join(i.userConfigsDir, "datadog-agent"))
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

		// Create a tmppath and set it as the root tmp directory
		tmpPath := filepath.Join(rootPath, "tmp")
		err := os.MkdirAll(tmpPath, 0755)
		assert.NoError(t, err)

		oldPurgeTmpDirectory := purgeTmpDirectory
		purgeTmpDirectory = func(_ string) error {
			err := os.RemoveAll(tmpPath)
			if err != nil {
				t.Fatalf("could not delete tmp directory: %v", err)
			}
			return nil
		}
		defer func() {
			purgeTmpDirectory = oldPurgeTmpDirectory
		}()

		// Create a file in the tmp directory
		err = os.WriteFile(filepath.Join(tmpPath, "test.txt"), []byte("test"), 0644)
		assert.NoError(t, err)

		err = instFactory(installer)(testCtx, s.PackageURL(fixtures.FixtureSimpleV1), nil)
		assert.NoError(t, err)
		r := installer.packages.Get(fixtures.FixtureSimpleV1.Package)

		state, err := r.GetState()
		assert.NoError(t, err)
		assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)

		installer.Purge(testCtx)
		assert.NoFileExists(t, filepath.Join(rootPath, "packages.db"), "purge should remove the packages database")
		assert.NoDirExists(t, rootPath, "purge should remove the packages directory")
		assert.Nil(t, installer.db, "purge should close the packages database")
		assert.NoDirExists(t, tmpPath, "purge should remove the tmp directory")
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
		"pkg/config/setup", // Needed for extensions
		"pkg/template",
	}

	// Walk the directory tree
	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only check .go files
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") {
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

// Test that only files older than 24 hours are deleted
func TestTmpDirectoryCleanup(t *testing.T) {
	tempDir := t.TempDir()

	oldFile := filepath.Join(tempDir, "old.txt")
	newFile := filepath.Join(tempDir, "new.txt")

	err := os.WriteFile(oldFile, []byte("old"), 0644)
	assert.NoError(t, err)

	err = os.WriteFile(newFile, []byte("new"), 0644)
	assert.NoError(t, err)

	oldTime := time.Now().Add(-25 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)

	err = os.Chtimes(oldFile, oldTime, oldTime)
	assert.NoError(t, err)

	err = os.Chtimes(newFile, newTime, newTime)
	assert.NoError(t, err)

	err = cleanupTmpDirectory(tempDir)
	assert.NoError(t, err)

	assert.NoFileExists(t, oldFile, "old file should be deleted")
	assert.FileExists(t, newFile, "new file should be kept")
}
