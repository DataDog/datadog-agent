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
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	updaterErrors "github.com/DataDog/datadog-agent/pkg/updater/errors"
	"github.com/DataDog/datadog-agent/pkg/updater/repository"
	"github.com/DataDog/datadog-agent/pkg/updater/service"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// defaultRepositoriesPath is the default path to the repositories directory.
	defaultRepositoriesPath = "/opt/datadog-packages"
	// defaultLocksPath is the default path to the run directory.
	defaultLocksPath = "/var/run/datadog-packages"
	// gcInterval is the interval at which the GC will run
	gcInterval = 1 * time.Hour
	// catalogOverridePath is the path to the catalog override file
	catalogOverridePath = defaultRepositoriesPath + "/catalog.json"
	// bootstrapVersionsOverridePath is the path to the bootstrap versions override file
	bootstrapVersionsOverridePath = defaultRepositoriesPath + "/bootstrap.json"
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

	Bootstrap(ctx context.Context, pkg string) error
	BootstrapVersion(ctx context.Context, pkg string, version string) error
	StartExperiment(ctx context.Context, pkg string, version string) error
	StopExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

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
	requests          chan remoteAPIRequest
	requestsWG        sync.WaitGroup
	bootstrapVersions bootstrapVersions
}

type disk interface {
	GetUsage(path string) (*filesystem.DiskUsage, error)
}

// Bootstrap bootstraps the default version for the given package.
func Bootstrap(ctx context.Context, pkg string, config config.Reader) error {
	rc := newNoopRemoteConfig()
	u := newUpdater(rc, defaultRepositoriesPath, defaultLocksPath, catalogOverridePath, bootstrapVersionsOverridePath, config)
	return u.Bootstrap(ctx, pkg)
}

// Purge removes files installed by the updater
func Purge() {
	purge(defaultLocksPath, defaultRepositoriesPath)
}

func purge(locksPath, repositoryPath string) {
	service.RemoveAgentUnits()
	cleanDir(locksPath, os.RemoveAll)
	cleanDir(repositoryPath, service.RemoveAll)
}

func cleanDir(dir string, cleanFunc func(string) error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warnf("updater: could not read directory %s: %v", dir, err)
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if path == catalogOverridePath || path == bootstrapVersionsOverridePath {
			continue
		}
		err := cleanFunc(path)
		if err != nil {
			log.Warnf("updater: could not remove %s: %v", path, err)
		}
	}
}

func getDefaultCatalog(catalogOverridePath string) (catalog, error) {
	var c catalog

	file, err := os.ReadFile(catalogOverridePath)
	if err != nil {
		return catalog{}, fmt.Errorf("could not read provided catalog file: %w", err)
	}

	err = json.Unmarshal(file, &c)
	if err != nil {
		return catalog{}, fmt.Errorf("could not unmarshal provided catalog file: %w", err)
	}

	return c, err
}

func getDefaultBootstrapVersions(bootstrapVersionsOverridePath string) (bootstrapVersions, error) {
	var b bootstrapVersions

	file, err := os.ReadFile(bootstrapVersionsOverridePath)
	if err != nil {
		return bootstrapVersions{}, fmt.Errorf("could not read provided catalog file: %w", err)
	}

	err = json.Unmarshal(file, &b)
	if err != nil {
		return bootstrapVersions{}, fmt.Errorf("could not unmarshal provided catalog file: %w", err)
	}

	return b, err
}

func getDefaults(catalogOverridePath string, bootstrapVersionsOverridePath string) (catalog catalog, versions bootstrapVersions, overridden bool) {
	catalog = defaultCatalog

	tmpCatalog, err := getDefaultCatalog(catalogOverridePath)
	if err != nil {
		log.Debug(fmt.Sprintf("could not read override catalog file: %s, falling back to default", err))
	} else {
		catalog = tmpCatalog
		overridden = true
	}

	versions = defaultBootstrapVersions
	tmpVersions, err := getDefaultBootstrapVersions(bootstrapVersionsOverridePath)
	if err != nil {
		log.Debug(fmt.Sprintf("could not read provided catalog file: %s, falling back to default", err))
	} else {
		versions = tmpVersions
		overridden = true
	}

	return
}

// NewUpdater returns a new Updater.
func NewUpdater(rcFetcher client.ConfigFetcher, config config.Reader) (Updater, error) {
	rc, err := newRemoteConfig(rcFetcher)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config client: %w", err)
	}
	return newUpdater(rc, defaultRepositoriesPath, defaultLocksPath, catalogOverridePath, bootstrapVersionsOverridePath, config), nil
}

func newUpdater(rc *remoteConfig, repositoriesPath string, locksPath string, catalogOverridePath string, bootstrapVersionsOverridePath string, config config.Reader) *updaterImpl {
	repositories := repository.NewRepositories(repositoriesPath, locksPath)
	remoteRegistryOverride := config.GetString("updater.registry")

	rcClient := rc
	catalog, defaultVersions, overridden := getDefaults(catalogOverridePath, bootstrapVersionsOverridePath)
	if overridden {
		log.Info("updater: catalog and/or default versions overridden, disabling remote config")
		rcClient = newNoopRemoteConfig()
	}

	u := &updaterImpl{
		rc:                rcClient,
		repositories:      repositories,
		downloader:        newDownloader(http.DefaultClient, remoteRegistryOverride),
		installer:         newInstaller(repositories),
		catalog:           catalog,
		requests:          make(chan remoteAPIRequest, 32),
		bootstrapVersions: defaultVersions,
		stopChan:          make(chan struct{}),
	}
	u.refreshState(context.Background())
	return u
}

// GetState returns the state.
func (u *updaterImpl) GetState() (map[string]repository.State, error) {
	return u.repositories.GetState()
}

// Start starts remote config and the garbage collector.
func (u *updaterImpl) Start(_ context.Context) error {
	u.rc.Start(u.handleCatalogUpdate, u.scheduleRemoteAPIRequest)
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
			case request := <-u.requests:
				err := u.handleRemoteAPIRequest(request)
				if err != nil {
					log.Errorf("updater: could not handle remote request: %v", err)
				}
			}
		}
	}()
	return nil
}

// Stop stops the garbage collector.
func (u *updaterImpl) Stop(_ context.Context) error {
	u.rc.Close()
	close(u.stopChan)
	u.requestsWG.Wait()
	close(u.requests)
	return nil
}

// Bootstrap installs the stable version of the package.
func (u *updaterImpl) Bootstrap(ctx context.Context, pkg string) error {
	u.m.Lock()
	defer u.m.Unlock()
	u.refreshState(ctx)
	defer u.refreshState(ctx)

	stablePackage, ok := u.catalog.getDefaultPackage(u.bootstrapVersions, pkg, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get default package %s for %s, %s", pkg, runtime.GOARCH, runtime.GOOS)
	}
	return u.boostrapPackage(ctx, stablePackage)
}

// Bootstrap installs the stable version of the package.
func (u *updaterImpl) BootstrapVersion(ctx context.Context, pkg string, version string) error {
	u.m.Lock()
	defer u.m.Unlock()
	u.refreshState(ctx)
	defer u.refreshState(ctx)

	stablePackage, ok := u.catalog.getPackage(pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get package %s version %s for %s, %s", pkg, version, runtime.GOARCH, runtime.GOOS)
	}
	return u.boostrapPackage(ctx, stablePackage)
}

func (u *updaterImpl) boostrapPackage(ctx context.Context, stablePackage Package) error {
	// both tmp and repository paths are checked for available disk space in case they are on different partitions
	err := checkAvailableDiskSpace(fsDisk, defaultRepositoriesPath, os.TempDir())
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
		return fmt.Errorf("could not download: %w", err)
	}
	err = u.installer.installStable(stablePackage.Name, stablePackage.Version, image)
	if err != nil {
		return fmt.Errorf("could not install: %w", err)
	}
	log.Infof("Updater: Successfully installed default version %s of package %s", stablePackage.Version, stablePackage.Name)
	return nil
}

// StartExperiment starts an experiment with the given package.
func (u *updaterImpl) StartExperiment(ctx context.Context, pkg string, version string) error {
	u.m.Lock()
	defer u.m.Unlock()
	u.refreshState(ctx)
	defer u.refreshState(ctx)

	log.Infof("Updater: Starting experiment for package %s version %s", pkg, version)
	// both tmp and repository paths are checked for available disk space in case they are on different partitions
	err := checkAvailableDiskSpace(fsDisk, defaultRepositoriesPath, os.TempDir())
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
func (u *updaterImpl) PromoteExperiment(ctx context.Context, pkg string) error {
	u.m.Lock()
	defer u.m.Unlock()
	u.refreshState(ctx)
	defer u.refreshState(ctx)

	log.Infof("Updater: Promoting experiment for package %s", pkg)
	err := u.installer.promoteExperiment(pkg)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	log.Infof("Updater: Successfully promoted experiment for package %s", pkg)
	return nil
}

// StopExperiment stops the experiment.
func (u *updaterImpl) StopExperiment(ctx context.Context, pkg string) error {
	u.m.Lock()
	defer u.m.Unlock()
	u.refreshState(ctx)
	defer u.refreshState(ctx)

	defer log.Infof("Updater: Stopping experiment for package %s", pkg)
	err := u.installer.uninstallExperiment(pkg)
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

func (u *updaterImpl) scheduleRemoteAPIRequest(request remoteAPIRequest) error {
	u.requestsWG.Add(1)
	u.requests <- request
	return nil
}

func (u *updaterImpl) handleRemoteAPIRequest(request remoteAPIRequest) (err error) {
	defer u.requestsWG.Done()
	ctx := newRequestContext(request)
	u.refreshState(ctx)
	defer u.refreshState(ctx)

	s, err := u.repositories.GetPackageState(request.Package)
	if err != nil {
		return fmt.Errorf("could not get updater state: %w", err)
	}
	if s.Stable != request.ExpectedState.Stable || s.Experiment != request.ExpectedState.Experiment {
		log.Infof("remote request %s not executed as state does not match: expected %v, got %v", request.ID, request.ExpectedState, s)
		setRequestInvalid(ctx)
		u.refreshState(ctx)
		return nil
	}

	defer func() { setRequestDone(ctx, err) }()
	switch request.Method {
	case methodStartExperiment:
		log.Infof("Updater: Received remote request %s to start experiment for package %s version %s", request.ID, request.Package, request.Params)
		var params taskWithVersionParams
		err := json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		return u.StartExperiment(context.Background(), request.Package, params.Version)
	case methodStopExperiment:
		log.Infof("Updater: Received remote request %s to stop experiment for package %s", request.ID, request.Package)
		return u.StopExperiment(ctx, request.Package)
	case methodPromoteExperiment:
		log.Infof("Updater: Received remote request %s to promote experiment for package %s", request.ID, request.Package)
		return u.PromoteExperiment(ctx, request.Package)
	case methodBootstrap:
		var params taskWithVersionParams
		err := json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		log.Infof("Updater: Received remote request %s to bootstrap package %s version %s", request.ID, request.Package, params.Version)
		if params.Version == "" {
			return u.Bootstrap(context.Background(), request.Package)
		}
		return u.BootstrapVersion(context.Background(), request.Package, params.Version)
	default:
		return fmt.Errorf("unknown method: %s", request.Method)
	}
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

type requestKey int

var requestStateKey requestKey

// requestState represents the state of a task.
type requestState struct {
	Package string
	ID      string
	State   pbgo.TaskState
	Err     *updaterErrors.UpdaterError
}

func newRequestContext(request remoteAPIRequest) context.Context {
	return context.WithValue(context.Background(), requestStateKey, &requestState{
		Package: request.Package,
		ID:      request.ID,
		State:   pbgo.TaskState_RUNNING,
	})
}

func setRequestInvalid(ctx context.Context) {
	state := ctx.Value(requestStateKey).(*requestState)
	state.State = pbgo.TaskState_INVALID_STATE
}

func setRequestDone(ctx context.Context, err error) {
	state := ctx.Value(requestStateKey).(*requestState)
	state.State = pbgo.TaskState_DONE
	if err != nil {
		state.State = pbgo.TaskState_ERROR
		state.Err = updaterErrors.From(err)
	}
}

func (u *updaterImpl) refreshState(ctx context.Context) {
	state, err := u.GetState()
	if err != nil {
		// TODO: we should report this error through RC in some way
		log.Errorf("could not get updater state: %v", err)
		return
	}
	requestState, ok := ctx.Value(requestStateKey).(*requestState)
	var packages []*pbgo.PackageState
	for pkg, s := range state {
		p := &pbgo.PackageState{
			Package:           pkg,
			StableVersion:     s.Stable,
			ExperimentVersion: s.Experiment,
		}
		if ok && pkg == requestState.Package {
			var taskErr *pbgo.TaskError
			if requestState.Err != nil {
				taskErr = &pbgo.TaskError{
					Code:    uint64(requestState.Err.Code()),
					Message: requestState.Err.Error(),
				}
			}
			p.Task = &pbgo.PackageStateTask{
				Id:    requestState.ID,
				State: requestState.State,
				Error: taskErr,
			}
		}
		packages = append(packages, p)
	}
	u.rc.SetState(packages)
}
