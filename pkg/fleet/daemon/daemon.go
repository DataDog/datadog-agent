// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package daemon implements the fleet long running daemon.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/service"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// gcInterval is the interval at which the GC will run
	gcInterval = 1 * time.Hour
)

// Daemon is the fleet daemon in charge of remote install, updates and configuration.
type Daemon interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	Install(ctx context.Context, url string) error
	StartExperiment(ctx context.Context, url string) error
	StopExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error

	GetPackage(pkg string, version string) (Package, error)
	GetState() (map[string]repository.State, error)
}

type daemonImpl struct {
	m        sync.Mutex
	stopChan chan struct{}

	packageManager installer.Installer
	remoteUpdates  bool
	rc             *remoteConfig
	catalog        catalog
	requests       chan remoteAPIRequest
	requestsWG     sync.WaitGroup
}

// BootstrapURL installs the given package from an URL.
func BootstrapURL(ctx context.Context, url string, config config.Reader) error {
	rc := newNoopRemoteConfig()
	i := newDaemon(rc, newPackageManager(config), false)
	err := i.Start(ctx)
	if err != nil {
		return fmt.Errorf("could not start daemon: %w", err)
	}
	defer func() {
		err := i.Stop(ctx)
		if err != nil {
			log.Errorf("could not stop daemon: %v", err)
		}
	}()
	return i.Install(ctx, url)
}

// Bootstrap is the generic installer bootstrap.
func Bootstrap(ctx context.Context, config config.Reader) error {
	rc := newNoopRemoteConfig()
	i := newDaemon(rc, newPackageManager(config), false)
	err := i.Start(ctx)
	if err != nil {
		return fmt.Errorf("could not start daemon: %w", err)
	}
	defer func() {
		err := i.Stop(ctx)
		if err != nil {
			log.Errorf("could not stop daemon: %v", err)
		}
	}()
	return i.Bootstrap(ctx)
}

// Remove removes an individual package
func Remove(ctx context.Context, pkg string) error {
	packageManager := installer.NewInstaller()
	return packageManager.Remove(ctx, pkg)
}

func newPackageManager(config config.Reader) installer.Installer {
	registry := config.GetString("updater.registry")
	registryAuth := config.GetString("updater.registry_auth")
	var opts []installer.Options
	if registry != "" {
		opts = append(opts, installer.WithRegistry(registry))
	}
	if registryAuth != "" {
		opts = append(opts, installer.WithRegistryAuth(installer.RegistryAuth(registryAuth)))
	}
	return installer.NewInstaller(opts...)
}

// NewDaemon returns a new daemon.
func NewDaemon(rcFetcher client.ConfigFetcher, config config.Reader) (Daemon, error) {
	rc, err := newRemoteConfig(rcFetcher)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config client: %w", err)
	}
	packagesManager := newPackageManager(config)
	remoteUpdates := config.GetBool("updater.remote_updates")
	return newDaemon(rc, packagesManager, remoteUpdates), nil
}

func newDaemon(rc *remoteConfig, packageManager installer.Installer, remoteUpdates bool) *daemonImpl {
	i := &daemonImpl{
		remoteUpdates:  remoteUpdates,
		rc:             rc,
		packageManager: packageManager,
		requests:       make(chan remoteAPIRequest, 32),
		catalog:        catalog{},
		stopChan:       make(chan struct{}),
	}
	i.refreshState(context.Background())
	return i
}

// GetState returns the state.
func (i *daemonImpl) GetState() (map[string]repository.State, error) {
	i.m.Lock()
	defer i.m.Unlock()

	return i.packageManager.States()
}

// GetPackage returns the package with the given name and version.
func (i *daemonImpl) GetPackage(pkg string, version string) (Package, error) {
	i.m.Lock()
	defer i.m.Unlock()

	catalogPackage, ok := i.catalog.getPackage(pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return Package{}, fmt.Errorf("could not get package %s, %s for %s, %s", pkg, version, runtime.GOARCH, runtime.GOOS)
	}
	return catalogPackage, nil
}

// Start starts remote config and the garbage collector.
func (i *daemonImpl) Start(_ context.Context) error {
	go func() {
		for {
			select {
			case <-time.After(gcInterval):
				i.m.Lock()
				err := i.packageManager.GarbageCollect(context.Background())
				i.m.Unlock()
				if err != nil {
					log.Errorf("Daemon: could not run GC: %v", err)
				}
			case <-i.stopChan:
				return
			case request := <-i.requests:
				err := i.handleRemoteAPIRequest(request)
				if err != nil {
					log.Errorf("Daemon: could not handle remote request: %v", err)
				}
			}
		}
	}()
	if !i.remoteUpdates {
		log.Infof("Daemon: Remote updates are disabled")
		return nil
	}
	i.rc.Start(i.handleCatalogUpdate, i.scheduleRemoteAPIRequest)
	return nil
}

// Stop stops the garbage collector.
func (i *daemonImpl) Stop(_ context.Context) error {
	i.rc.Close()
	close(i.stopChan)
	i.requestsWG.Wait()
	close(i.requests)
	return nil
}

// Bootstrap is the generic bootstrap of the installer
func (i *daemonImpl) Bootstrap(ctx context.Context) (err error) {
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

// Install installs the package from the given URL.
func (i *daemonImpl) Install(ctx context.Context, url string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "start_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	log.Infof("Daemon: Bootstrapping package from %s", url)
	err = i.packageManager.Install(ctx, url)
	if err != nil {
		return fmt.Errorf("could not install: %w", err)
	}
	log.Infof("Daemon: Successfully installed package from %s", url)
	return nil
}

// StartExperiment starts an experiment with the given package.
func (i *daemonImpl) StartExperiment(ctx context.Context, url string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "start_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	log.Infof("Daemon: Starting experiment for package from %s", url)
	err = i.packageManager.InstallExperiment(ctx, url)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	log.Infof("Daemon: Successfully started experiment for package from %s", url)
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (i *daemonImpl) PromoteExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "promote_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	log.Infof("Daemon: Promoting experiment for package %s", pkg)
	err = i.packageManager.PromoteExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	log.Infof("Daemon: Successfully promoted experiment for package %s", pkg)
	return nil
}

// StopExperiment stops the experiment.
func (i *daemonImpl) StopExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "stop_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	log.Infof("Daemon: Stopping experiment for package %s", pkg)
	err = i.packageManager.RemoveExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not stop experiment: %w", err)
	}
	log.Infof("Daemon: Successfully stopped experiment for package %s", pkg)
	return nil
}

func (i *daemonImpl) setupInstallerUnits(ctx context.Context) (err error) {
	systemdRunning, err := service.IsSystemdRunning()
	if err != nil {
		return fmt.Errorf("error checking if systemd is running: %w", err)
	}
	if !systemdRunning {
		log.Infof("Daemon: Systemd is not running, skipping unit setup")
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

func (i *daemonImpl) handleCatalogUpdate(c catalog) error {
	i.m.Lock()
	defer i.m.Unlock()
	log.Infof("Daemon: Received catalog update")
	i.catalog = c
	return nil
}

func (i *daemonImpl) scheduleRemoteAPIRequest(request remoteAPIRequest) error {
	i.requestsWG.Add(1)
	i.requests <- request
	return nil
}

func (i *daemonImpl) handleRemoteAPIRequest(request remoteAPIRequest) (err error) {
	i.m.Lock()
	defer i.m.Unlock()
	defer i.requestsWG.Done()
	ctx := newRequestContext(request)
	i.refreshState(ctx)
	defer i.refreshState(ctx)

	s, err := i.packageManager.State(request.Package)
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

	i.m.Unlock()
	switch request.Method {
	case methodStartExperiment:
		log.Infof("Daemon: Received remote request %s to start experiment for package %s version %s", request.ID, request.Package, request.Params)
		err = i.remoteAPIStartExperiment(ctx, request)
	case methodStopExperiment:
		log.Infof("Daemon: Received remote request %s to stop experiment for package %s", request.ID, request.Package)
		err = i.StopExperiment(ctx, request.Package)
	case methodPromoteExperiment:
		log.Infof("Daemon: Received remote request %s to promote experiment for package %s", request.ID, request.Package)
		err = i.PromoteExperiment(ctx, request.Package)
	default:
		err = fmt.Errorf("unknown method: %s", request.Method)
	}
	i.m.Lock()
	return err
}

func (i *daemonImpl) remoteAPIStartExperiment(ctx context.Context, request remoteAPIRequest) error {
	var params taskWithVersionParams
	err := json.Unmarshal(request.Params, &params)
	if err != nil {
		return fmt.Errorf("could not unmarshal start experiment params: %w", err)
	}
	experimentPackage, ok := i.catalog.getPackage(request.Package, params.Version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return fmt.Errorf("could not get package %s, %s for %s, %s", request.Package, params.Version, runtime.GOARCH, runtime.GOOS)
	}
	return i.StartExperiment(ctx, experimentPackage.URL)
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

func (i *daemonImpl) refreshState(ctx context.Context) {
	state, err := i.packageManager.States()
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
