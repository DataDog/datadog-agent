// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer implements the installer.
package installer

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

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	installerErrors "github.com/DataDog/datadog-agent/pkg/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/installer/service"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
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
)

var (
	// requiredDiskSpace is the required disk space to download and extract a package
	// It is the sum of the maximum size of the extracted oci-layout and the maximum size of the datadog package
	requiredDiskSpace = ociLayoutMaxSize + datadogPackageMaxSize
	fsDisk            = filesystem.NewDisk()
)

// Installer is the datadog packages installer.
type Installer interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	BootstrapURL(ctx context.Context, url string) error
	BootstrapDefault(ctx context.Context, pkg string) error
	BootstrapVersion(ctx context.Context, pkg string, version string) error
	StartExperiment(ctx context.Context, pkg string, version string) error
	StopExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	GetState() (map[string]repository.State, error)
}

type installerImpl struct {
	m        sync.Mutex
	stopChan chan struct{}

	repositories   *repository.Repositories
	downloader     *downloader
	packageManager *packageManager

	remoteUpdates     bool
	rc                *remoteConfig
	catalog           catalog
	requests          chan remoteAPIRequest
	requestsWG        sync.WaitGroup
	bootstrapVersions bootstrapVersions
}

type disk interface {
	GetUsage(path string) (*filesystem.DiskUsage, error)
}

// BootstrapURL installs the given package from an URL.
func BootstrapURL(ctx context.Context, url string, config config.Reader) error {
	rc := newNoopRemoteConfig()
	i, err := newInstaller(rc, defaultRepositoriesPath, defaultLocksPath, config)
	if err != nil {
		return fmt.Errorf("could not create installer: %w", err)
	}
	err = i.Start(ctx)
	if err != nil {
		return fmt.Errorf("could not start installer: %w", err)
	}
	defer func() {
		err := i.Stop(ctx)
		if err != nil {
			log.Errorf("could not stop installer: %v", err)
		}
	}()
	return i.BootstrapURL(ctx, url)
}

// Bootstrap is the generic installer bootstrap.
func Bootstrap(ctx context.Context, config config.Reader) error {
	rc := newNoopRemoteConfig()
	i, err := newInstaller(rc, defaultRepositoriesPath, defaultLocksPath, config)
	if err != nil {
		return fmt.Errorf("could not create installer: %w", err)
	}
	err = i.Start(ctx)
	if err != nil {
		return fmt.Errorf("could not start installer: %w", err)
	}
	defer func() {
		err := i.Stop(ctx)
		if err != nil {
			log.Errorf("could not stop installer: %v", err)
		}
	}()
	return i.Bootstrap(ctx)
}

// Purge removes files installed by the installer
func Purge() {
	purge(defaultLocksPath, defaultRepositoriesPath)
}

// PurgePackage removes an individual package
func PurgePackage(pkg string) {
	purgePackage(pkg, defaultLocksPath, defaultRepositoriesPath)
}

func purgePackage(pkg string, locksPath, repositoryPath string) {
	var err error
	span, ctx := tracer.StartSpanFromContext(context.Background(), "purge_pkg")
	defer span.Finish(tracer.WithError(err))
	span.SetTag("params.pkg", pkg)

	switch pkg {
	case "datadog-installer":
		service.RemoveInstallerUnits(ctx)
	case "datadog-agent":
		service.RemoveAgentUnits(ctx)
	case "datadog-apm-inject":
		if err = service.RemoveAPMInjector(ctx); err != nil {
			log.Warnf("installer: could not remove APM injector: %v", err)
		}
	default:
		log.Warnf("installer: unrecognized package purge")
		return
	}
	if err = removePkgDirs(ctx, pkg, locksPath, repositoryPath); err != nil {
		log.Warnf("installer: %v", err)
	}
}

func purge(locksPath, repositoryPath string) {
	var err error
	span, ctx := tracer.StartSpanFromContext(context.Background(), "purge")
	defer span.Finish(tracer.WithError(err))

	service.RemoveInstallerUnits(ctx)

	// todo(paullgdc): The APM injection removal already checks that the LD_PRELOAD points to the injector
	// in /opt/datadog-packages before removal, so this should not impact previous deb installations
	// of the injector, but since customers won't use install apm injectors, this codepath is not useful
	// so it's safer to comment it out until we decide to install the apm-injector on boostrap

	// if err = service.RemoveAPMInjector(ctx); err != nil {
	// 	log.Warnf("installer: could not remove APM injector: %v", err)
	// }

	cleanDir(locksPath, os.RemoveAll)
	cleanDir(repositoryPath, func(path string) error { return service.RemoveAll(ctx, path) })
}

func removePkgDirs(ctx context.Context, pkg string, locksPath, repositoryPath string) (err error) {
	pkgLockPath := filepath.Join(locksPath, pkg)
	if lockPathErr := os.RemoveAll(pkgLockPath); lockPathErr != nil {
		err = fmt.Errorf("could not remove %s: %w", pkgLockPath, lockPathErr)
	}

	pkgRepositoryPath := filepath.Join(repositoryPath, pkg)
	if pkgRepositoryErr := service.RemoveAll(ctx, pkgRepositoryPath); err != nil {
		err = fmt.Errorf("%w; could not remove %s: %w", err, pkgRepositoryPath, pkgRepositoryErr)
	}
	return err
}

func cleanDir(dir string, cleanFunc func(string) error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warnf("installer: could not read directory %s: %v", dir, err)
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		err := cleanFunc(path)
		if err != nil {
			log.Warnf("installer: could not remove %s: %v", path, err)
		}
	}
}

// NewInstaller returns a new Installer.
func NewInstaller(rcFetcher client.ConfigFetcher, config config.Reader) (Installer, error) {
	rc, err := newRemoteConfig(rcFetcher)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config client: %w", err)
	}
	return newInstaller(rc, defaultRepositoriesPath, defaultLocksPath, config)
}

func newInstaller(rc *remoteConfig, repositoriesPath string, locksPath string, config config.Reader) (*installerImpl, error) {
	repositories := repository.NewRepositories(repositoriesPath, locksPath)
	remoteRegistryOverride := config.GetString("updater.registry")
	rcClient := rc
	i := &installerImpl{
		remoteUpdates:     config.GetBool("updater.remote_updates"),
		rc:                rcClient,
		repositories:      repositories,
		downloader:        newDownloader(config, http.DefaultClient, remoteRegistryOverride),
		packageManager:    newPackageManager(repositories),
		requests:          make(chan remoteAPIRequest, 32),
		catalog:           catalog{},
		bootstrapVersions: bootstrapVersions{},
		stopChan:          make(chan struct{}),
	}
	i.refreshState(context.Background())
	return i, nil
}

// GetState returns the state.
func (i *installerImpl) GetState() (map[string]repository.State, error) {
	return i.repositories.GetState()
}

// Start starts remote config and the garbage collector.
func (i *installerImpl) Start(_ context.Context) error {
	go func() {
		for {
			select {
			case <-time.After(gcInterval):
				i.m.Lock()
				err := i.repositories.Cleanup(context.Background())
				i.m.Unlock()
				if err != nil {
					log.Errorf("installer: could not run GC: %v", err)
				}
			case <-i.stopChan:
				return
			case request := <-i.requests:
				err := i.handleRemoteAPIRequest(request)
				if err != nil {
					log.Errorf("installer: could not handle remote request: %v", err)
				}
			}
		}
	}()
	if !i.remoteUpdates {
		log.Infof("installer: Remote updates are disabled")
		return nil
	}
	i.rc.Start(i.handleCatalogUpdate, i.scheduleRemoteAPIRequest)
	return nil
}

// Stop stops the garbage collector.
func (i *installerImpl) Stop(_ context.Context) error {
	i.rc.Close()
	close(i.stopChan)
	i.requestsWG.Wait()
	close(i.requests)
	return nil
}

// Bootstrap installs the stable version of the package.
func (i *installerImpl) BootstrapDefault(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "bootrap_default")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	stablePackage, ok := i.catalog.getDefaultPackage(i.bootstrapVersions, pkg, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get default package '%s' for arch '%s' and platform '%s'", pkg, runtime.GOARCH, runtime.GOOS)
	}
	return i.bootstrapPackage(ctx, stablePackage.URL, stablePackage.Name, stablePackage.Version)
}

// BootstrapVersion installs the stable version of the package.
func (i *installerImpl) BootstrapVersion(ctx context.Context, pkg string, version string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "bootstrap_version")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	stablePackage, ok := i.catalog.getPackage(pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get package '%s' version '%s' for arch '%s' and platform '%s'", pkg, version, runtime.GOARCH, runtime.GOOS)
	}
	return i.bootstrapPackage(ctx, stablePackage.URL, stablePackage.Name, stablePackage.Version)
}

// Bootstrap is the generic bootstrap of the installer
func (i *installerImpl) Bootstrap(ctx context.Context) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "bootstrap")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	if err = i.setupInstallerUnits(ctx); err != nil {
		return err
	}

	return nil
}

// BootstrapURL installs the stable version of the package.
func (i *installerImpl) BootstrapURL(ctx context.Context, url string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "bootstrap_url")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	return i.bootstrapPackage(ctx, url, "", "")
}

func (i *installerImpl) bootstrapPackage(ctx context.Context, url string, expectedPackage string, expectedVersion string) error {
	// both tmp and repository paths are checked for available disk space in case they are on different partitions
	err := checkAvailableDiskSpace(fsDisk, defaultRepositoriesPath, os.TempDir())
	if err != nil {
		return fmt.Errorf("not enough disk space to install package: %w", err)
	}
	log.Infof("Installer: Bootstrapping stable package from %s", url)
	downloadedPackage, err := i.downloader.Download(ctx, url)
	if err != nil {
		return fmt.Errorf("could not download: %w", err)
	}
	// check that the downloaded package metadata matches the catalog metadata
	if (expectedPackage != "" && downloadedPackage.Name != expectedPackage) || (expectedVersion != "" && downloadedPackage.Version != expectedVersion) {
		return fmt.Errorf("downloaded package does not match expected package: %s, %s != %s, %s", downloadedPackage.Name, downloadedPackage.Version, expectedPackage, expectedVersion)
	}
	err = i.packageManager.installStable(ctx, downloadedPackage.Name, downloadedPackage.Version, downloadedPackage.Image)
	if err != nil {
		return fmt.Errorf("could not install: %w", err)
	}

	log.Infof("Installer: Successfully installed default version %s of package %s from %s", downloadedPackage.Version, downloadedPackage.Name, url)
	return nil
}

// StartExperiment starts an experiment with the given package.
func (i *installerImpl) StartExperiment(ctx context.Context, pkg string, version string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "bootstrap_version")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	log.Infof("Installer: Starting experiment for package %s version %s", pkg, version)
	// both tmp and repository paths are checked for available disk space in case they are on different partitions
	err = checkAvailableDiskSpace(fsDisk, defaultRepositoriesPath, os.TempDir())
	if err != nil {
		return fmt.Errorf("not enough disk space to install package: %w", err)
	}
	experimentPackage, ok := i.catalog.getPackage(pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get package %s, %s for %s, %s", pkg, version, runtime.GOARCH, runtime.GOOS)
	}
	downloadedPackage, err := i.downloader.Download(ctx, experimentPackage.URL)
	if err != nil {
		return fmt.Errorf("could not download experiment: %w", err)
	}
	// check that the downloaded package metadata matches the catalog metadata
	if downloadedPackage.Name != experimentPackage.Name || downloadedPackage.Version != experimentPackage.Version {
		return fmt.Errorf("downloaded package does not match requested package: %s, %s != %s, %s", downloadedPackage.Name, downloadedPackage.Version, experimentPackage.Name, experimentPackage.Version)
	}
	err = i.packageManager.installExperiment(ctx, pkg, version, downloadedPackage.Image)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	log.Infof("Installer: Successfully started experiment for package %s version %s", pkg, version)
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (i *installerImpl) PromoteExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "promote_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	log.Infof("Installer: Promoting experiment for package %s", pkg)
	err = i.packageManager.promoteExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	log.Infof("Installer: Successfully promoted experiment for package %s", pkg)
	return nil
}

// StopExperiment stops the experiment.
func (i *installerImpl) StopExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "stop_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	defer log.Infof("Installer: Stopping experiment for package %s", pkg)
	err = i.packageManager.uninstallExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not stop experiment: %w", err)
	}
	log.Infof("Installer: Successfully stopped experiment for package %s", pkg)
	return nil
}

func (i *installerImpl) setupInstallerUnits(ctx context.Context) (err error) {
	systemdRunning, err := service.IsSystemdRunning()
	if err != nil {
		return fmt.Errorf("error checking if systemd is running: %w", err)
	}
	if !systemdRunning {
		log.Infof("Installer: Systemd is not running, skipping unit setup")
		return nil
	}
	err = service.SetupInstallerUnits(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup datadog-installer systemd units: %w", err)
	}
	if !i.remoteUpdates {
		service.RemoveInstallerUnits(ctx)
		return
	}
	return service.StartInstallerStable(ctx)
}

func (i *installerImpl) handleCatalogUpdate(c catalog) error {
	i.m.Lock()
	defer i.m.Unlock()
	log.Infof("Installer: Received catalog update")
	i.catalog = c
	return nil
}

func (i *installerImpl) scheduleRemoteAPIRequest(request remoteAPIRequest) error {
	i.requestsWG.Add(1)
	i.requests <- request
	return nil
}

func (i *installerImpl) handleRemoteAPIRequest(request remoteAPIRequest) (err error) {
	defer i.requestsWG.Done()
	ctx := newRequestContext(request)
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	s, err := i.repositories.GetPackageState(request.Package)
	if err != nil {
		return fmt.Errorf("could not get installer state: %w", err)
	}
	if s.Stable != request.ExpectedState.Stable || s.Experiment != request.ExpectedState.Experiment {
		log.Infof("remote request %s not executed as state does not match: expected %v, got %v", request.ID, request.ExpectedState, s)
		setRequestInvalid(ctx)
		i.refreshState(ctx)
		return nil
	}

	defer func() { setRequestDone(ctx, err) }()
	switch request.Method {
	case methodStartExperiment:
		log.Infof("Installer: Received remote request %s to start experiment for package %s version %s", request.ID, request.Package, request.Params)
		var params taskWithVersionParams
		err := json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		return i.StartExperiment(context.Background(), request.Package, params.Version)
	case methodStopExperiment:
		log.Infof("Installer: Received remote request %s to stop experiment for package %s", request.ID, request.Package)
		return i.StopExperiment(ctx, request.Package)
	case methodPromoteExperiment:
		log.Infof("Installer: Received remote request %s to promote experiment for package %s", request.ID, request.Package)
		return i.PromoteExperiment(ctx, request.Package)
	case methodBootstrap:
		var params taskWithVersionParams
		err := json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		log.Infof("Installer: Received remote request %s to bootstrap package %s version %s", request.ID, request.Package, params.Version)
		if params.Version == "" {
			return i.BootstrapDefault(context.Background(), request.Package)
		}
		return i.BootstrapVersion(context.Background(), request.Package, params.Version)
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
	Err     *installerErrors.InstallerError
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
		state.Err = installerErrors.From(err)
	}
}

func (i *installerImpl) refreshState(ctx context.Context) {
	state, err := i.GetState()
	if err != nil {
		// TODO: we should report this error through RC in some way
		log.Errorf("could not get installer state: %v", err)
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
	i.rc.SetState(packages)
}
