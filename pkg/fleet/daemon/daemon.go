// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package daemon implements the fleet long running daemon.
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/bootstrap"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/cdn"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// gcInterval is the interval at which the GC will run
	gcInterval = 1 * time.Hour
	// refreshStateInterval is the interval at which the state will be refreshed
	refreshStateInterval = 30 * time.Second
)

// Daemon is the fleet daemon in charge of remote install, updates and configuration.
type Daemon interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	SetCatalog(c catalog)
	Install(ctx context.Context, url string, args []string) error
	StartExperiment(ctx context.Context, url string) error
	StopExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error
	StartConfigExperiment(ctx context.Context, pkg string, hash string) error
	StopConfigExperiment(ctx context.Context, pkg string) error
	PromoteConfigExperiment(ctx context.Context, pkg string) error

	GetPackage(pkg string, version string) (Package, error)
	GetState() (map[string]repository.State, error)
	GetRemoteConfigState() *pbgo.ClientUpdater
	GetAPMInjectionStatus() (APMInjectionStatus, error)
}

type daemonImpl struct {
	m        sync.Mutex
	stopChan chan struct{}

	env           *env.Env
	installer     installer.Installer
	rc            *remoteConfig
	cdn           cdn.CDN
	catalog       catalog
	requests      chan remoteAPIRequest
	requestsWG    sync.WaitGroup
	requestsState map[string]requestState
}

func newInstaller(env *env.Env, installerBin string) installer.Installer {
	return exec.NewInstallerExec(env, installerBin)
}

// NewDaemon returns a new daemon.
func NewDaemon(rcFetcher client.ConfigFetcher, config config.Reader) (Daemon, error) {
	installerBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("could not get installer executable path: %w", err)
	}
	installerBin, err = filepath.EvalSymlinks(installerBin)
	if err != nil {
		return nil, fmt.Errorf("could not get resolve installer executable path: %w", err)
	}
	rc, err := newRemoteConfig(rcFetcher)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config client: %w", err)
	}
	env := env.FromConfig(config)
	installer := newInstaller(env, installerBin)
	cdn, err := cdn.New(env, filepath.Join(paths.RunPath, "rc_daemon"))
	if err != nil {
		return nil, err
	}
	return newDaemon(rc, installer, env, cdn), nil
}

func newDaemon(rc *remoteConfig, installer installer.Installer, env *env.Env, cdn cdn.CDN) *daemonImpl {
	i := &daemonImpl{
		env:           env,
		rc:            rc,
		installer:     installer,
		cdn:           cdn,
		requests:      make(chan remoteAPIRequest, 32),
		catalog:       catalog{},
		stopChan:      make(chan struct{}),
		requestsState: make(map[string]requestState),
	}
	i.refreshState(context.Background())
	return i
}

// GetState returns the state.
func (d *daemonImpl) GetState() (map[string]repository.State, error) {
	d.m.Lock()
	defer d.m.Unlock()

	return d.installer.States()
}

// GetRemoteConfigState returns the remote config state.
func (d *daemonImpl) GetRemoteConfigState() *pbgo.ClientUpdater {
	d.m.Lock()
	defer d.m.Unlock()

	return d.rc.GetState()
}

// GetAPMInjectionStatus returns the APM injection status. This is not done in the service
// to avoid cross-contamination between the daemon and the installer.
func (d *daemonImpl) GetAPMInjectionStatus() (status APMInjectionStatus, err error) {
	d.m.Lock()
	defer d.m.Unlock()

	// Host is instrumented if the ld.so.preload file contains the apm injector
	ldPreloadContent, err := os.ReadFile("/etc/ld.so.preload")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return status, fmt.Errorf("could not read /etc/ld.so.preload: %w", err)
	}
	if bytes.Contains(ldPreloadContent, []byte("/opt/datadog-packages/datadog-apm-inject/stable/inject")) {
		status.HostInstrumented = true
	}

	// Docker is installed if the docker binary is in the PATH
	_, err = osexec.LookPath("docker")
	if err != nil && errors.Is(err, osexec.ErrNotFound) {
		return status, nil
	} else if err != nil {
		return status, fmt.Errorf("could not check if docker is installed: %w", err)
	}
	status.DockerInstalled = true

	// Docker is instrumented if there is the injector runtime in its configuration
	// We're not retrieving the default runtime from the docker daemon as we are not
	// root
	dockerConfigContent, err := os.ReadFile("/etc/docker/daemon.json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return status, fmt.Errorf("could not read /etc/docker/daemon.json: %w", err)
	} else if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if bytes.Contains(dockerConfigContent, []byte("/opt/datadog-packages/datadog-apm-inject/stable/inject")) {
		status.DockerInstrumented = true
	}

	return status, nil
}

// GetPackage returns the package with the given name and version.
func (d *daemonImpl) GetPackage(pkg string, version string) (Package, error) {
	d.m.Lock()
	defer d.m.Unlock()

	catalogPackage, ok := d.catalog.getPackage(pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return Package{}, fmt.Errorf("could not get package %s, %s for %s, %s", pkg, version, runtime.GOARCH, runtime.GOOS)
	}
	return catalogPackage, nil
}

// SetCatalog sets the catalog.
func (d *daemonImpl) SetCatalog(c catalog) {
	d.m.Lock()
	defer d.m.Unlock()
	d.catalog = c
}

// Start starts remote config and the garbage collector.
func (d *daemonImpl) Start(_ context.Context) error {
	d.m.Lock()
	defer d.m.Unlock()
	go func() {
		gcTicker := time.NewTicker(gcInterval)
		defer gcTicker.Stop()
		refreshStateTicker := time.NewTicker(refreshStateInterval)
		defer refreshStateTicker.Stop()
		for {
			select {
			case <-gcTicker.C:
				d.m.Lock()
				err := d.installer.GarbageCollect(context.Background())
				d.m.Unlock()
				if err != nil {
					log.Errorf("Daemon: could not run GC: %v", err)
				}
			case <-refreshStateTicker.C:
				d.m.Lock()
				d.refreshState(context.Background())
				d.m.Unlock()
			case <-d.stopChan:
				return
			case request := <-d.requests:
				err := d.handleRemoteAPIRequest(request)
				if err != nil {
					log.Errorf("Daemon: could not handle remote request: %v", err)
				}
			}
		}
	}()
	if !d.env.RemoteUpdates {
		log.Infof("Daemon: Remote updates are disabled")
		return nil
	}
	d.rc.Start(d.handleCatalogUpdate, d.scheduleRemoteAPIRequest)
	return nil
}

// Stop stops the garbage collector.
func (d *daemonImpl) Stop(_ context.Context) error {
	d.m.Lock()
	defer d.m.Unlock()
	d.rc.Close()
	close(d.stopChan)
	d.cdn.Close()
	d.requestsWG.Wait()
	return nil
}

// Install installs the package from the given URL.
func (d *daemonImpl) Install(ctx context.Context, url string, args []string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.install(ctx, url, args)
}

func (d *daemonImpl) install(ctx context.Context, url string, args []string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "install")
	defer func() { span.Finish(tracer.WithError(err)) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Installing package from %s", url)
	err = d.installer.Install(ctx, url, args)
	if err != nil {
		return fmt.Errorf("could not install: %w", err)
	}
	log.Infof("Daemon: Successfully installed package from %s", url)
	return nil
}

// StartExperiment starts an experiment with the given package.
func (d *daemonImpl) StartExperiment(ctx context.Context, url string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.startExperiment(ctx, url)
}

func (d *daemonImpl) startExperiment(ctx context.Context, url string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "start_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Starting experiment for package from %s", url)
	err = d.installer.InstallExperiment(ctx, url)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	log.Infof("Daemon: Successfully started experiment for package from %s", url)
	return nil
}

func (d *daemonImpl) startInstallerExperiment(ctx context.Context, url string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "start_installer_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Starting installer experiment for package from %s", url)
	if runtime.GOOS == "windows" {
		err = d.installer.InstallExperiment(ctx, url)
	} else {
		err = bootstrap.InstallExperiment(ctx, d.env, url)
	}
	if err != nil {
		return fmt.Errorf("could not install installer experiment: %w", err)
	}
	log.Infof("Daemon: Successfully started installer experiment for package from %s", url)
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (d *daemonImpl) PromoteExperiment(ctx context.Context, pkg string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.promoteExperiment(ctx, pkg)
}

func (d *daemonImpl) promoteExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "promote_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Promoting experiment for package %s", pkg)
	err = d.installer.PromoteExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	log.Infof("Daemon: Successfully promoted experiment for package %s", pkg)
	return nil
}

// StopExperiment stops the experiment.
func (d *daemonImpl) StopExperiment(ctx context.Context, pkg string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.stopExperiment(ctx, pkg)
}

func (d *daemonImpl) stopExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "stop_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Stopping experiment for package %s", pkg)
	err = d.installer.RemoveExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not stop experiment: %w", err)
	}
	log.Infof("Daemon: Successfully stopped experiment for package %s", pkg)
	return nil
}

// StartConfigExperiment starts a config experiment with the given package.
func (d *daemonImpl) StartConfigExperiment(ctx context.Context, url string, version string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.startConfigExperiment(ctx, url, version)
}

func (d *daemonImpl) startConfigExperiment(ctx context.Context, url string, version string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "start_config_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Starting config experiment for package from %s", url)
	err = d.installer.InstallConfigExperiment(ctx, url, version)
	if err != nil {
		return fmt.Errorf("could not start config experiment: %w", err)
	}
	log.Infof("Daemon: Successfully started config experiment for package from %s", url)
	return nil
}

// PromoteConfigExperiment promotes the experiment to stable.
func (d *daemonImpl) PromoteConfigExperiment(ctx context.Context, pkg string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.promoteConfigExperiment(ctx, pkg)
}

func (d *daemonImpl) promoteConfigExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "promote_config_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Promoting config experiment for package %s", pkg)
	err = d.installer.PromoteConfigExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not promote config experiment: %w", err)
	}
	log.Infof("Daemon: Successfully promoted config experiment for package %s", pkg)
	return nil
}

// StopConfigExperiment stops the experiment.
func (d *daemonImpl) StopConfigExperiment(ctx context.Context, pkg string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.stopConfigExperiment(ctx, pkg)
}

func (d *daemonImpl) stopConfigExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "stop_config_experiment")
	defer func() { span.Finish(tracer.WithError(err)) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Stopping config experiment for package %s", pkg)
	err = d.installer.RemoveConfigExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not stop config experiment: %w", err)
	}
	log.Infof("Daemon: Successfully stopped config experiment for package %s", pkg)
	return nil
}

func (d *daemonImpl) handleCatalogUpdate(c catalog) error {
	d.m.Lock()
	defer d.m.Unlock()
	log.Infof("Installer: Received catalog update")
	d.catalog = c
	return nil
}

func (d *daemonImpl) scheduleRemoteAPIRequest(request remoteAPIRequest) error {
	d.requestsWG.Add(1)
	d.requests <- request
	return nil
}

func (d *daemonImpl) handleRemoteAPIRequest(request remoteAPIRequest) (err error) {
	d.m.Lock()
	defer d.m.Unlock()
	defer d.requestsWG.Done()
	parentSpan, ctx := newRequestContext(request)
	defer parentSpan.Finish(tracer.WithError(err))
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	s, err := d.installer.State(request.Package)
	if err != nil {
		return fmt.Errorf("could not get installer state: %w", err)
	}

	c, err := d.installer.ConfigState(request.Package)
	if err != nil {
		return fmt.Errorf("could not get installer config state: %w", err)
	}

	versionEqual := request.ExpectedState.InstallerVersion == "" || version.AgentVersion == request.ExpectedState.InstallerVersion
	if versionEqual &&
		(s.Stable != request.ExpectedState.Stable ||
			s.Experiment != request.ExpectedState.Experiment ||
			c.Stable != request.ExpectedState.StableConfig ||
			c.Experiment != request.ExpectedState.ExperimentConfig) {
		log.Infof("remote request %s not executed as state does not match: expected %v, got %v", request.ID, request.ExpectedState, s)
		setRequestInvalid(ctx)
		d.refreshState(ctx)
		return nil
	}
	defer func() { setRequestDone(ctx, err) }()

	switch request.Method {
	case methodStartExperiment:
		var params taskWithVersionParams
		err = json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		experimentPackage, ok := d.catalog.getPackage(request.Package, params.Version, runtime.GOARCH, runtime.GOOS)
		if !ok {
			return fmt.Errorf("could not get package %s, %s for %s, %s", request.Package, params.Version, runtime.GOARCH, runtime.GOOS)
		}
		log.Infof("Installer: Received remote request %s to start experiment for package %s version %s", request.ID, request.Package, request.Params)
		if request.Package == "datadog-installer" {
			// Special case for the installer package as we want the experiment installer to start the experiment itself
			return d.startInstallerExperiment(ctx, experimentPackage.URL)
		}
		return d.startExperiment(ctx, experimentPackage.URL)
	case methodStopExperiment:
		log.Infof("Installer: Received remote request %s to stop experiment for package %s", request.ID, request.Package)
		return d.stopExperiment(ctx, request.Package)
	case methodPromoteExperiment:
		log.Infof("Installer: Received remote request %s to promote experiment for package %s", request.ID, request.Package)
		return d.promoteExperiment(ctx, request.Package)

	case methodStartConfigExperiment:
		var params taskWithVersionParams
		err = json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		log.Infof("Installer: Received remote request %s to start config experiment for package %s", request.ID, request.Package)
		return d.startConfigExperiment(ctx, request.Package, params.Version)
	case methodStopConfigExperiment:
		log.Infof("Installer: Received remote request %s to stop config experiment for package %s", request.ID, request.Package)
		return d.stopConfigExperiment(ctx, request.Package)
	case methodPromoteConfigExperiment:
		log.Infof("Installer: Received remote request %s to promote config experiment for package %s", request.ID, request.Package)
		return d.promoteConfigExperiment(ctx, request.Package)

	default:
		return fmt.Errorf("unknown method: %s", request.Method)
	}
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

func newRequestContext(request remoteAPIRequest) (ddtrace.Span, context.Context) {
	ctx := context.WithValue(context.Background(), requestStateKey, &requestState{
		Package: request.Package,
		ID:      request.ID,
		State:   pbgo.TaskState_RUNNING,
	})

	ctxCarrier := tracer.TextMapCarrier{
		tracer.DefaultTraceIDHeader:  request.TraceID,
		tracer.DefaultParentIDHeader: request.ParentSpanID,
		tracer.DefaultPriorityHeader: "2",
	}
	spanCtx, err := tracer.Extract(ctxCarrier)
	if err != nil {
		log.Debugf("failed to extract span context from install script params: %v", err)
		return tracer.StartSpan("remote_request"), ctx
	}

	return tracer.StartSpanFromContext(ctx, "remote_request", tracer.ChildOf(spanCtx))
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

func (d *daemonImpl) resolveRemoteConfigVersion(ctx context.Context, pkg string) (string, error) {
	if !d.env.RemotePolicies {
		return "", nil
	}
	config, err := d.cdn.Get(ctx, pkg)
	if err != nil {
		return "", err
	}
	return config.Version(), nil
}

func (d *daemonImpl) refreshState(ctx context.Context) {
	request, ok := ctx.Value(requestStateKey).(*requestState)
	if ok {
		d.requestsState[request.Package] = *request
	}
	state, err := d.installer.States()
	if err != nil {
		// TODO: we should report this error through RC in some way
		log.Errorf("could not get installer state: %v", err)
		return
	}
	configState, err := d.installer.ConfigStates()
	if err != nil {
		log.Errorf("could not get installer config state: %v", err)
		return
	}
	availableSpace, err := d.installer.AvailableDiskSpace()
	if err != nil {
		log.Errorf("could not get available size: %v", err)
	}

	var packages []*pbgo.PackageState
	for pkg, s := range state {
		p := &pbgo.PackageState{
			Package:           pkg,
			StableVersion:     s.Stable,
			ExperimentVersion: s.Experiment,
		}
		cs, hasConfig := configState[pkg]
		if hasConfig {
			p.StableConfigVersion = cs.Stable
			p.ExperimentConfigVersion = cs.Experiment
		}

		configVersion, err := d.resolveRemoteConfigVersion(ctx, pkg)
		if err == nil {
			p.RemoteConfigVersion = configVersion
		} else if err != cdn.ErrProductNotSupported {
			log.Warnf("could not get remote config version: %v", err)
		}

		requestState, ok := d.requestsState[pkg]
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
	d.rc.SetState(&pbgo.ClientUpdater{
		Packages:           packages,
		AvailableDiskSpace: availableSpace,
	})
}
