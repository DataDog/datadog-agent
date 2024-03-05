// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater implements the updater.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	updaterErrors "github.com/DataDog/datadog-agent/pkg/updater/errors"
	"github.com/DataDog/datadog-agent/pkg/updater/repository"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/uuid"
)

const (
	// defaultRepositoriesPath is the default path to the repositories directory.
	defaultRepositoriesPath = "/opt/datadog-packages"
	// defaultLocksPath is the default path to the run directory.
	defaultLocksPath = "/var/run/datadog-packages"
	// gcInterval is the interval at which the GC will run
	gcInterval = 1 * time.Hour
)

var (
	// requiredDiskSpace is the required disk space to download and extract a package
	// It is the sum of the maximum size of the extracted oci-layout and the maximum size of the datadog package
	requiredDiskSpace = ociLayoutMaxSize + datadogPackageMaxSize
	fsDisk            = filesystem.NewDisk()
)

// Updater is the updater used to update packages.
type Updater interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	Bootstrap(ctx context.Context, pkg string, taskID string) error
	BootstrapVersion(ctx context.Context, pkg string, version string, taskID string) error
	StartExperiment(ctx context.Context, pkg string, version string, taskID string) error
	StopExperiment(pkg string, taskID string) error
	PromoteExperiment(pkg string, taskID string) error

	GetState() (map[string]repository.State, error)
}

type updaterImpl struct {
	m        sync.Mutex
	stopChan chan struct{}

	repositories *repository.Repositories
	downloader   *downloader
	installer    *installer

	rc                *remoteConfig
	catalog           catalog
	bootstrapVersions bootstrapVersions
}

// TaskState represents the state of a task.
type TaskState struct {
	ID    string
	State pbgo.TaskState
	Err   *updaterErrors.UpdaterError
}

type disk interface {
	GetUsage(path string) (*filesystem.DiskUsage, error)
}

// Bootstrap bootstraps the default version for the given package.
func Bootstrap(ctx context.Context, pkg string) error {
	rc := newNoopRemoteConfig()
	u, err := newUpdater(rc, defaultRepositoriesPath, defaultLocksPath)
	if err != nil {
		return err
	}
	taskID := uuid.New().String()
	return u.Bootstrap(ctx, pkg, taskID)
}

// BootstrapVersion bootstraps the given package at the given version.
func BootstrapVersion(ctx context.Context, pkg string, version string) error {
	rc := newNoopRemoteConfig()
	u, err := newUpdater(rc, defaultRepositoriesPath, defaultLocksPath)
	if err != nil {
		return err
	}
	taskID := uuid.New().String()
	return u.BootstrapVersion(ctx, pkg, version, taskID)
}

// NewUpdater returns a new Updater.
func NewUpdater(rcFetcher client.ConfigFetcher) (Updater, error) {
	rc, err := newRemoteConfig(rcFetcher)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config client: %w", err)
	}
	return newUpdater(rc, defaultRepositoriesPath, defaultLocksPath)
}

func newUpdater(rc *remoteConfig, repositoriesPath string, locksPath string) (*updaterImpl, error) {
	repositories := repository.NewRepositories(repositoriesPath, locksPath)
	u := &updaterImpl{
		rc:                rc,
		repositories:      repositories,
		downloader:        newDownloader(http.DefaultClient),
		installer:         newInstaller(repositories),
		catalog:           defaultCatalog,
		bootstrapVersions: defaultBootstrapVersions,
		stopChan:          make(chan struct{}),
	}
	state, err := u.GetState()
	if err != nil {
		return nil, fmt.Errorf("could not get updater state: %w", err)
	}
	initTaskState := TaskState{
		State: pbgo.TaskState_IDLE,
	}
	for pkg, s := range state {
		u.rc.SetState(pkg, s, initTaskState)
	}
	return u, nil
}

// GetState returns the state.
func (u *updaterImpl) GetState() (map[string]repository.State, error) {
	return u.repositories.GetState()
}

// Start starts remote config and the garbage collector.
func (u *updaterImpl) Start(_ context.Context) error {
	u.rc.Start(u.handleCatalogUpdate, u.handleRemoteAPIRequest)
	go func() {
		for {
			select {
			case <-time.After(gcInterval):
				u.m.Lock()
				err := u.repositories.Cleanup()
				u.m.Unlock()
				if err != nil {
					log.Errorf("updater: could not run GC: %v", err)
				}
			case <-u.stopChan:
				return
			}
		}
	}()
	return nil
}

// Stop stops the garbage collector.
func (u *updaterImpl) Stop(_ context.Context) error {
	u.rc.Close()
	close(u.stopChan)
	return nil
}

// Bootstrap installs the stable version of the package.
func (u *updaterImpl) Bootstrap(ctx context.Context, pkg string, taskID string) error {
	u.m.Lock()
	defer u.m.Unlock()
	stablePackage, ok := u.catalog.getDefaultPackage(u.bootstrapVersions, pkg, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get default package %s for %s, %s", pkg, runtime.GOARCH, runtime.GOOS)
	}
	return u.boostrapPackage(ctx, stablePackage, taskID)
}

// Bootstrap installs the stable version of the package.
func (u *updaterImpl) BootstrapVersion(ctx context.Context, pkg string, version string, taskID string) error {
	u.m.Lock()
	defer u.m.Unlock()
	stablePackage, ok := u.catalog.getPackage(pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get package %s version %s for %s, %s", pkg, version, runtime.GOARCH, runtime.GOOS)
	}
	return u.boostrapPackage(ctx, stablePackage, taskID)
}

func (u *updaterImpl) boostrapPackage(ctx context.Context, stablePackage Package, taskID string) error {
	var err error
	u.setPackagesStateTaskRunning(stablePackage.Name, taskID)
	defer func() {
		u.setPackagesStateTaskFinished(stablePackage.Name, taskID, err)
	}()

	// both tmp and repository paths are checked for available disk space in case they are on different partitions
	err = checkAvailableDiskSpace(fsDisk, defaultRepositoriesPath, os.TempDir())
	if err != nil {
		return fmt.Errorf("not enough disk space to install package: %w", err)
	}
	log.Infof("Updater: Bootstrapping stable version %s of package %s", stablePackage.Version, stablePackage.Name)
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	image, err := u.downloader.Download(ctx, tmpDir, stablePackage)
	if err != nil {
		return fmt.Errorf("could not download experiment: %w", err)
	}
	err = u.installer.installStable(stablePackage.Name, stablePackage.Version, image)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	log.Infof("Updater: Successfully installed default version %s of package %s", stablePackage.Version, stablePackage.Name)
	return nil
}

// StartExperiment starts an experiment with the given package.
func (u *updaterImpl) StartExperiment(ctx context.Context, pkg string, version string, taskID string) error {
	u.m.Lock()
	defer u.m.Unlock()

	var err error
	u.setPackagesStateTaskRunning(pkg, taskID)
	defer func() {
		u.setPackagesStateTaskFinished(pkg, taskID, err)
	}()

	log.Infof("Updater: Starting experiment for package %s version %s", pkg, version)
	// both tmp and repository paths are checked for available disk space in case they are on different partitions
	err = checkAvailableDiskSpace(fsDisk, defaultRepositoriesPath, os.TempDir())
	if err != nil {
		return fmt.Errorf("not enough disk space to install package: %w", err)
	}
	experimentPackage, ok := u.catalog.getPackage(pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get package %s, %s for %s, %s", pkg, version, runtime.GOARCH, runtime.GOOS)
	}
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	image, err := u.downloader.Download(ctx, tmpDir, experimentPackage)
	if err != nil {
		return fmt.Errorf("could not download experiment: %w", err)
	}
	err = u.installer.installExperiment(pkg, version, image)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	log.Infof("Updater: Successfully started experiment for package %s version %s", pkg, version)
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (u *updaterImpl) PromoteExperiment(pkg string, taskID string) error {
	u.m.Lock()
	defer u.m.Unlock()

	var err error
	u.setPackagesStateTaskRunning(pkg, taskID)
	defer func() {
		u.setPackagesStateTaskFinished(pkg, taskID, err)
	}()

	log.Infof("Updater: Promoting experiment for package %s", pkg)
	err = u.installer.promoteExperiment(pkg)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	log.Infof("Updater: Successfully promoted experiment for package %s", pkg)
	return nil
}

// StopExperiment stops the experiment.
func (u *updaterImpl) StopExperiment(pkg string, taskID string) error {
	u.m.Lock()
	defer u.m.Unlock()

	var err error
	u.setPackagesStateTaskRunning(pkg, taskID)
	defer func() {
		u.setPackagesStateTaskFinished(pkg, taskID, err)
	}()

	log.Infof("Updater: Stopping experiment for package %s", pkg)
	err = u.installer.uninstallExperiment(pkg)
	if err != nil {
		return fmt.Errorf("could not stop experiment: %w", err)
	}
	log.Infof("Updater: Successfully stopped experiment for package %s", pkg)
	return nil
}

func (u *updaterImpl) handleCatalogUpdate(c catalog) error {
	u.m.Lock()
	defer u.m.Unlock()
	log.Infof("Updater: Received catalog update")
	u.catalog = c
	return nil
}

func (u *updaterImpl) handleRemoteAPIRequest(request remoteAPIRequest) error {
	s, err := u.repositories.GetPackageState(request.Package)
	if err != nil {
		return fmt.Errorf("could not get updater state: %w", err)
	}
	if s.Stable != request.ExpectedState.Stable || s.Experiment != request.ExpectedState.Experiment {
		u.setPackagesStateInvalid(request.Package, request.ID)
		log.Infof("remote request %s not executed as state does not match: expected %v, got %v", request.ID, request.ExpectedState, s)
		return nil
	}

	switch request.Method {
	case methodStartExperiment:
		log.Infof("Updater: Received remote request %s to start experiment for package %s version %s", request.ID, request.Package, request.Params)
		var params taskWithVersionParams
		err := json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		return u.StartExperiment(context.Background(), request.Package, params.Version, request.ID)
	case methodStopExperiment:
		log.Infof("Updater: Received remote request %s to stop experiment for package %s", request.ID, request.Package)
		return u.StopExperiment(request.Package, request.ID)
	case methodPromoteExperiment:
		log.Infof("Updater: Received remote request %s to promote experiment for package %s", request.ID, request.Package)
		return u.PromoteExperiment(request.Package, request.ID)
	case methodBootstrap:
		var params taskWithVersionParams
		err := json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		log.Infof("Updater: Received remote request %s to bootstrap package %s version %s", request.ID, request.Package, params.Version)
		if params.Version == "" {
			return u.Bootstrap(context.Background(), request.Package, request.ID)
		}
		return u.BootstrapVersion(context.Background(), request.Package, params.Version, request.ID)
	default:
		return fmt.Errorf("unknown method: %s", request.Method)
	}
}

// setPackagesStateTaskRunning sets the packages state to RUNNING.
// and is called before a task starts
func (u *updaterImpl) setPackagesStateTaskRunning(pkg string, taskID string) {
	taskState := TaskState{
		ID:    taskID,
		State: pbgo.TaskState_RUNNING,
	}

	repoState, err := u.repositories.GetPackageState(pkg)
	if err != nil {
		log.Warnf("could not update packages state: %s", err)
		return
	}
	u.rc.SetState(pkg, repoState, taskState)
}

// setPackagesStateTaskFinished sets the packages state once a task is done
// depending on the taskErr, the state will be set to DONE or ERROR
func (u *updaterImpl) setPackagesStateTaskFinished(pkg string, taskID string, taskErr error) {
	taskState := TaskState{
		ID:    taskID,
		State: pbgo.TaskState_DONE,
		Err:   updaterErrors.From(taskErr),
	}
	if taskErr != nil {
		taskState.State = pbgo.TaskState_ERROR
	}

	repoState, err := u.repositories.GetPackageState(pkg)
	if err != nil {
		log.Warnf("could not update packages state: %s", err)
		return
	}
	u.rc.SetState(pkg, repoState, taskState)
}

// setPackagesStateInvalid sets the packages state to INVALID,
// if the stable or experiment version does not match the expected state
func (u *updaterImpl) setPackagesStateInvalid(pkg string, taskID string) {
	taskState := TaskState{
		ID:    taskID,
		State: pbgo.TaskState_INVALID_STATE,
	}

	repoState, err := u.repositories.GetPackageState(pkg)
	if err != nil {
		log.Warnf("could not update packages state: %s", err)
		return
	}
	u.rc.SetState(pkg, repoState, taskState)
}

// checkAvailableDiskSpace checks if there is enough disk space to download and extract a package in the given paths.
// This will check the underlying partition of the given path. Note that the path must be an existing dir.
//
// On Unix, it is computed using `statfs` and is the number of free blocks available to an unprivileged used * block size
// See https://man7.org/linux/man-pages/man2/statfs.2.html for more details
// On Windows, it is computed using `GetDiskFreeSpaceExW` and is the number of bytes available
// See https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw for more details
func checkAvailableDiskSpace(fsDisk disk, paths ...string) error {
	for _, path := range paths {
		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("could not stat path %s: %w", path, err)
		}
		s, err := fsDisk.GetUsage(path)
		if err != nil {
			return err
		}
		if s.Available < uint64(requiredDiskSpace) {
			return fmt.Errorf("not enough disk space to download package: %d bytes available at %s, %d required", s.Available, path, requiredDiskSpace)
		}
	}
	return nil
}
