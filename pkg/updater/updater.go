// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater implements the updater.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"

	"github.com/DataDog/datadog-agent/pkg/updater/repository"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// defaultRepositoryPath is the default path to the repository.
	defaultRepositoryPath = "/opt/datadog-packages"
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

	StartExperiment(ctx context.Context, version string) error
	StopExperiment() error
	PromoteExperiment() error

	GetRepositoryPath() string
	GetPackage() string
	GetState() (*repository.State, error)
}

type updaterImpl struct {
	m        sync.Mutex
	stopChan chan struct{}

	pkg            string
	repositoryPath string
	repository     *repository.Repository
	downloader     *downloader
	installer      *installer

	rc                *remoteConfig
	catalog           catalog
	bootstrapVersions bootstrapVersions
}

type disk interface {
	GetUsage(path string) (*filesystem.DiskUsage, error)
}

// Bootstrap bootstraps the default version for the given package.
// It is purposefully not part of the updater to avoid misuse.
func Bootstrap(ctx context.Context, pkg string) error {
	repository := &repository.Repository{
		RootPath:  path.Join(defaultRepositoryPath, pkg),
		LocksPath: path.Join(defaultLocksPath, pkg),
	}
	rc := newNoopRemoteConfig()
	u := newUpdater(rc, repository, pkg)
	return u.bootstrapStable(ctx)
}

// NewUpdater returns a new Updater.
func NewUpdater(rcFetcher client.ConfigFetcher, pkg string) (Updater, error) {
	rc, err := newRemoteConfig(rcFetcher)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config client: %w", err)
	}
	repository := &repository.Repository{
		RootPath:  path.Join(defaultRepositoryPath, pkg),
		LocksPath: path.Join(defaultLocksPath, pkg),
	}
	return newUpdater(rc, repository, pkg), nil
}

func newUpdater(rc *remoteConfig, repository *repository.Repository, pkg string) *updaterImpl {
	u := &updaterImpl{
		pkg:               pkg,
		repositoryPath:    defaultRepositoryPath,
		rc:                rc,
		repository:        repository,
		downloader:        newDownloader(http.DefaultClient),
		installer:         newInstaller(repository),
		catalog:           defaultCatalog,
		bootstrapVersions: defaultBootstrapVersions,
		stopChan:          make(chan struct{}),
	}
	u.updatePackagesState()
	return u
}

// GetRepositoryPath returns the path to the repository.
func (u *updaterImpl) GetRepositoryPath() string {
	return u.repositoryPath
}

// GetPackage returns the package.
func (u *updaterImpl) GetPackage() string {
	return u.pkg
}

// GetState returns the state.
func (u *updaterImpl) GetState() (*repository.State, error) {
	return u.repository.GetState()
}

// Start starts remote config and the garbage collector.
func (u *updaterImpl) Start(_ context.Context) error {
	u.rc.Start(u.handleCatalogUpdate, u.handleRemoteAPIRequest)
	go func() {
		for {
			select {
			case <-time.After(gcInterval):
				u.m.Lock()
				err := u.repository.Cleanup()
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

// bootstrapStable installs the stable version of the package.
func (u *updaterImpl) bootstrapStable(ctx context.Context) error {
	u.m.Lock()
	defer u.m.Unlock()
	// both tmp and repository paths are checked for available disk space in case they are on different partitions
	err := checkAvailableDiskSpace(fsDisk, defaultRepositoryPath, os.TempDir())
	if err != nil {
		return fmt.Errorf("not enough disk space to install package: %w", err)
	}
	stablePackage, ok := u.catalog.getDefaultPackage(u.bootstrapVersions, u.pkg, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get default package %s for %s, %s", u.pkg, runtime.GOARCH, runtime.GOOS)
	}
	log.Infof("Updater: Installing default version %s of package %s", stablePackage.Version, u.pkg)
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	image, err := u.downloader.Download(ctx, tmpDir, stablePackage)
	if err != nil {
		return fmt.Errorf("could not download experiment: %w", err)
	}
	err = u.installer.installStable(stablePackage.Version, image)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	log.Infof("Updater: Successfully installed default version %s of package %s", stablePackage.Version, u.pkg)
	return nil
}

// StartExperiment starts an experiment with the given package.
func (u *updaterImpl) StartExperiment(ctx context.Context, version string) error {
	u.m.Lock()
	defer u.m.Unlock()
	log.Infof("Updater: Starting experiment for package %s version %s", u.pkg, version)
	// both tmp and repository paths are checked for available disk space in case they are on different partitions
	err := checkAvailableDiskSpace(fsDisk, u.repository.RootPath, os.TempDir())
	if err != nil {
		return fmt.Errorf("not enough disk space to install package: %w", err)
	}
	experimentPackage, ok := u.catalog.getPackage(u.pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get package %s, %s for %s, %s", u.pkg, version, runtime.GOARCH, runtime.GOOS)
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
	err = u.installer.installExperiment(version, image)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	log.Infof("Updater: Successfully started experiment for package %s version %s", u.pkg, version)
	u.updatePackagesState()
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (u *updaterImpl) PromoteExperiment() error {
	u.m.Lock()
	defer u.m.Unlock()
	log.Infof("Updater: Promoting experiment for package %s", u.pkg)
	err := u.installer.promoteExperiment()
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	log.Infof("Updater: Successfully promoted experiment for package %s", u.pkg)
	u.updatePackagesState()
	return nil
}

// StopExperiment stops the experiment.
func (u *updaterImpl) StopExperiment() error {
	u.m.Lock()
	defer u.m.Unlock()
	log.Infof("Updater: Stopping experiment for package %s", u.pkg)
	err := u.installer.uninstallExperiment()
	if err != nil {
		return fmt.Errorf("could not stop experiment: %w", err)
	}
	log.Infof("Updater: Successfully stopped experiment for package %s", u.pkg)
	u.updatePackagesState()
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
	s, err := u.GetState()
	if err != nil {
		return fmt.Errorf("could not get updater state: %w", err)
	}
	if s.Stable != request.ExpectedState.Stable || s.Experiment != request.ExpectedState.Experiment {
		log.Infof("remote request %s not executed as state does not match: expected %v, got %v", request.ID, request.ExpectedState, s)
		return nil
	}
	switch request.Method {
	case methodStartExperiment:
		log.Infof("Updater: Received remote request %s to start experiment for package %s version %s", request.ID, u.pkg, request.Params)
		var params startExperimentParams
		err := json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		return u.StartExperiment(context.Background(), params.Version)
	case methodStopExperiment:
		log.Infof("Updater: Received remote request %s to stop experiment for package %s", request.ID, u.pkg)
		return u.StopExperiment()
	case methodPromoteExperiment:
		log.Infof("Updater: Received remote request %s to promote experiment for package %s", request.ID, u.pkg)
		return u.PromoteExperiment()
	default:
		return fmt.Errorf("unknown method: %s", request.Method)
	}
}

func (u *updaterImpl) updatePackagesState() {
	state, err := u.repository.GetState()
	if err != nil {
		log.Warnf("could not update packages state: %s", err)
		return
	}
	u.rc.SetState(u.pkg, state)
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
