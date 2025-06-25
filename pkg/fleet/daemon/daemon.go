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
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/bootstrap"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
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

var (
	// errStateDoesntMatch is the error returned when the state doesn't match
	errStateDoesntMatch = errors.New("state doesn't match")

	// installExperimentFunc is the method to install an experiment. Overridden in tests.
	installExperimentFunc = bootstrap.InstallExperiment
)

// PackageState represents a package state.
type PackageState struct {
	Version repository.State
	Config  repository.State
}

// Daemon is the fleet daemon in charge of remote install, updates and configuration.
type Daemon interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	SetCatalog(c catalog)
	SetConfigCatalog(configs map[string]installerConfig)
	Install(ctx context.Context, url string, args []string) error
	Remove(ctx context.Context, pkg string) error
	StartExperiment(ctx context.Context, url string) error
	StopExperiment(ctx context.Context, pkg string) error
	PromoteExperiment(ctx context.Context, pkg string) error
	StartConfigExperiment(ctx context.Context, pkg string, hash string) error
	StopConfigExperiment(ctx context.Context, pkg string) error
	PromoteConfigExperiment(ctx context.Context, pkg string) error

	GetPackage(pkg string, version string) (Package, error)
	GetState(ctx context.Context) (map[string]PackageState, error)
	GetRemoteConfigState() *pbgo.ClientUpdater
	GetAPMInjectionStatus() (APMInjectionStatus, error)
}

type daemonImpl struct {
	m        sync.Mutex
	stopChan chan struct{}

	env             *env.Env
	installer       func(*env.Env) installer.Installer
	rc              *remoteConfig
	catalog         catalog
	catalogOverride catalog
	configs         map[string]installerConfig
	configsOverride map[string]installerConfig
	requests        chan remoteAPIRequest
	requestsWG      sync.WaitGroup
	taskDB          *taskDB
}

func newInstaller(installerBin string) func(env *env.Env) installer.Installer {
	return func(env *env.Env) installer.Installer {
		return exec.NewInstallerExec(env, installerBin)
	}
}

// NewDaemon returns a new daemon.
func NewDaemon(hostname string, rcFetcher client.ConfigFetcher, config config.Reader) (Daemon, error) {
	installerBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("could not get installer executable path: %w", err)
	}
	installerBin, err = filepath.EvalSymlinks(installerBin)
	if err != nil {
		return nil, fmt.Errorf("could not get resolve installer executable path: %w", err)
	}
	dbPath := filepath.Join(paths.RunPath, "installer_tasks.db")
	taskDB, err := newTaskDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("could not create task DB: %w", err)
	}
	rc, err := newRemoteConfig(rcFetcher)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config client: %w", err)
	}
	env := &env.Env{
		APIKey:               utils.SanitizeAPIKey(config.GetString("api_key")),
		Site:                 config.GetString("site"),
		RemoteUpdates:        config.GetBool("remote_updates"),
		Mirror:               config.GetString("installer.mirror"),
		RegistryOverride:     config.GetString("installer.registry.url"),
		RegistryAuthOverride: config.GetString("installer.registry.auth"),
		RegistryUsername:     config.GetString("installer.registry.username"),
		RegistryPassword:     config.GetString("installer.registry.password"),
		Tags:                 utils.GetConfiguredTags(config, false),
		Hostname:             hostname,
		HTTPProxy:            config.GetString("proxy.http"),
		HTTPSProxy:           config.GetString("proxy.https"),
		NoProxy:              strings.Join(config.GetStringSlice("proxy.no_proxy"), ","),
		IsCentos6:            env.DetectCentos6(),
		IsFromDaemon:         true,
	}
	installer := newInstaller(installerBin)
	return newDaemon(rc, installer, env, taskDB), nil
}

func newDaemon(rc *remoteConfig, installer func(env *env.Env) installer.Installer, env *env.Env, taskDB *taskDB) *daemonImpl {
	i := &daemonImpl{
		env:             env,
		rc:              rc,
		installer:       installer,
		requests:        make(chan remoteAPIRequest, 32),
		catalog:         catalog{},
		catalogOverride: catalog{},
		configs:         make(map[string]installerConfig),
		configsOverride: make(map[string]installerConfig),
		stopChan:        make(chan struct{}),
		taskDB:          taskDB,
	}
	i.refreshState(context.Background())
	return i
}

// GetState returns the state.
func (d *daemonImpl) GetState(ctx context.Context) (map[string]PackageState, error) {
	d.m.Lock()
	defer d.m.Unlock()

	states, err := d.installer(d.env).States(ctx)
	if err != nil {
		return nil, err
	}

	var configStates map[string]repository.State
	configStates, err = d.installer(d.env).ConfigStates(ctx)
	if err != nil {
		return nil, err
	}

	res := make(map[string]PackageState)
	for pkg := range states {
		res[pkg] = PackageState{
			Version: states[pkg],
			Config:  configStates[pkg],
		}
	}
	return res, nil
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

	return d.getPackage(pkg, version)
}

func (d *daemonImpl) getPackage(pkg string, version string) (Package, error) {
	catalog := d.catalog
	if len(d.catalogOverride.Packages) > 0 {
		catalog = d.catalogOverride
	}
	catalogPackage, ok := catalog.getPackage(pkg, version, runtime.GOARCH, runtime.GOOS)
	if !ok {
		return Package{}, fmt.Errorf("could not get package %s, %s for %s, %s", pkg, version, runtime.GOARCH, runtime.GOOS)
	}
	return catalogPackage, nil
}

// SetCatalog sets the catalog.
func (d *daemonImpl) SetCatalog(c catalog) {
	d.m.Lock()
	defer d.m.Unlock()
	d.catalogOverride = c
}

// SetConfigCatalog sets the config catalog override.
func (d *daemonImpl) SetConfigCatalog(configs map[string]installerConfig) {
	d.m.Lock()
	defer d.m.Unlock()
	d.configsOverride = configs
}

// Start starts remote config and the garbage collector.
func (d *daemonImpl) Start(_ context.Context) error {
	d.m.Lock()
	defer d.m.Unlock()

	if !d.env.RemoteUpdates {
		// If remote updates are disabled, we don't need to start the daemon
		return nil
	}

	go func() {
		gcTicker := time.NewTicker(gcInterval)
		defer gcTicker.Stop()
		refreshStateTicker := time.NewTicker(refreshStateInterval)
		defer refreshStateTicker.Stop()
		for {
			select {
			case <-gcTicker.C:
				d.m.Lock()
				err := d.installer(d.env).GarbageCollect(context.Background())
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
	d.rc.Start(d.handleConfigsUpdate, d.handleCatalogUpdate, d.scheduleRemoteAPIRequest)
	return nil
}

// Stop stops the garbage collector.
func (d *daemonImpl) Stop(_ context.Context) error {
	d.m.Lock()
	defer d.m.Unlock()

	if !d.env.RemoteUpdates {
		// If remote updates are disabled, we don't need to stop the daemon as it was never started
		return nil
	}

	d.rc.Close()
	close(d.stopChan)
	d.requestsWG.Wait()
	return d.taskDB.Close()
}

// Install installs the package from the given URL.
func (d *daemonImpl) Install(ctx context.Context, url string, args []string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.install(ctx, d.env, url, args)
}

func (d *daemonImpl) install(ctx context.Context, env *env.Env, url string, args []string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "install")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Installing package from %s", url)
	err = d.installer(env).Install(ctx, url, args)
	if err != nil {
		return fmt.Errorf("could not install: %w", err)
	}
	log.Infof("Daemon: Successfully installed package from %s", url)
	return nil
}

func (d *daemonImpl) Remove(ctx context.Context, pkg string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.remove(ctx, pkg)
}

func (d *daemonImpl) remove(ctx context.Context, pkg string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "remove")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Removing package %s", pkg)
	err = d.installer(d.env).Remove(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not remove: %w", err)
	}
	log.Infof("Daemon: Successfully removed package %s", pkg)
	return nil
}

// StartExperiment starts an experiment with the given package.
func (d *daemonImpl) StartExperiment(ctx context.Context, url string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.startExperiment(ctx, url)
}

func (d *daemonImpl) startExperiment(ctx context.Context, url string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "start_experiment")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Starting experiment for package from %s", url)
	err = installExperimentFunc(ctx, d.env, url)
	if err != nil {
		return fmt.Errorf("could not install experiment: %w", err)
	}
	log.Infof("Daemon: Successfully started experiment for package from %s", url)
	return nil
}

// PromoteExperiment promotes the experiment to stable.
func (d *daemonImpl) PromoteExperiment(ctx context.Context, pkg string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.promoteExperiment(ctx, pkg)
}

func (d *daemonImpl) promoteExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "promote_experiment")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Promoting experiment for package %s", pkg)
	err = d.installer(d.env).PromoteExperiment(ctx, pkg)
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
	span, ctx := telemetry.StartSpanFromContext(ctx, "stop_experiment")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Stopping experiment for package %s", pkg)
	err = d.installer(d.env).RemoveExperiment(ctx, pkg)
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

func (d *daemonImpl) startConfigExperiment(ctx context.Context, pkg string, version string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "start_config_experiment")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Starting config experiment version %s for package %s", version, pkg)
	configs := d.configs
	if len(d.configsOverride) > 0 {
		configs = d.configsOverride
	}
	config, ok := configs[version]
	if !ok {
		return fmt.Errorf("could not find config version %s", version)
	}
	serializedConfigFiles, err := json.Marshal(config.Files)
	if err != nil {
		return fmt.Errorf("could not serialize config files: %w", err)
	}
	err = d.installer(d.env).InstallConfigExperiment(ctx, pkg, version, serializedConfigFiles)
	if err != nil {
		return fmt.Errorf("could not start config experiment: %w", err)
	}
	log.Infof("Daemon: Successfully started config experiment version %s for package %s", version, pkg)
	return nil
}

// PromoteConfigExperiment promotes the experiment to stable.
func (d *daemonImpl) PromoteConfigExperiment(ctx context.Context, pkg string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.promoteConfigExperiment(ctx, pkg)
}

func (d *daemonImpl) promoteConfigExperiment(ctx context.Context, pkg string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "promote_config_experiment")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Promoting config experiment for package %s", pkg)
	err = d.installer(d.env).PromoteConfigExperiment(ctx, pkg)
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
	span, ctx := telemetry.StartSpanFromContext(ctx, "stop_config_experiment")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Stopping config experiment for package %s", pkg)
	err = d.installer(d.env).RemoveConfigExperiment(ctx, pkg)
	if err != nil {
		return fmt.Errorf("could not stop config experiment: %w", err)
	}
	log.Infof("Daemon: Successfully stopped config experiment for package %s", pkg)
	return nil
}

func (d *daemonImpl) handleConfigsUpdate(configs map[string]installerConfig) error {
	d.m.Lock()
	defer d.m.Unlock()
	log.Infof("Installer: Received configs update")
	d.configs = configs
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
	defer parentSpan.Finish(err)
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	err = d.verifyState(ctx, request)
	if err != nil {
		if errors.Is(err, errStateDoesntMatch) {
			return nil // Error already reported to RC
		}
		return fmt.Errorf("couldn't verify state: %w", err)
	}

	defer func() { setRequestDone(ctx, err) }()

	switch request.Method {
	case methodInstallPackage:
		var params installPackageTaskParams
		err = json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal install package params: %w", err)
		}
		log.Infof("Installer: Received remote request %s to install package %s version %s", request.ID, request.Package, params.Version)

		// Handle install args
		newEnv := *d.env
		if params.ApmInstrumentation != "" {
			if err := env.ValidateAPMInstrumentationEnabled(params.ApmInstrumentation); err != nil {
				return fmt.Errorf("invalid APM instrumentation value: %w", err)
			}
			newEnv.InstallScript.APMInstrumentationEnabled = params.ApmInstrumentation
		}

		pkg, err := d.getPackage(request.Package, params.Version)
		if err != nil {
			return installerErrors.Wrap(
				installerErrors.ErrPackageNotFound,
				err,
			)
		}
		return d.install(ctx, &newEnv, pkg.URL, nil)

	case methodUninstallPackage:
		log.Infof("Installer: Received remote request %s to uninstall package %s", request.ID, request.Package)
		if request.Package == "datadog-installer" || request.Package == "datadog-agent" {
			log.Infof("Installer: Can't uninstall the package %s", request.Package)
			return nil
		}
		return d.remove(ctx, request.Package)

	case methodStartExperiment:
		var params experimentTaskParams
		err = json.Unmarshal(request.Params, &params)
		if err != nil {
			return fmt.Errorf("could not unmarshal start experiment params: %w", err)
		}
		experimentPackage, ok := d.catalog.getPackage(request.Package, params.Version, runtime.GOARCH, runtime.GOOS)
		if !ok {
			return installerErrors.Wrap(
				installerErrors.ErrPackageNotFound,
				fmt.Errorf("could not get package %s, %s for %s, %s", request.Package, params.Version, runtime.GOARCH, runtime.GOOS),
			)
		}
		log.Infof("Installer: Received remote request %s to start experiment for package %s version %s", request.ID, request.Package, request.Params)
		return d.startExperiment(ctx, experimentPackage.URL)

	case methodStopExperiment:
		log.Infof("Installer: Received remote request %s to stop experiment for package %s", request.ID, request.Package)
		return d.stopExperiment(ctx, request.Package)

	case methodPromoteExperiment:
		log.Infof("Installer: Received remote request %s to promote experiment for package %s", request.ID, request.Package)
		return d.promoteExperiment(ctx, request.Package)

	case methodStartConfigExperiment:
		var params experimentTaskParams
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

func (d *daemonImpl) verifyState(ctx context.Context, request remoteAPIRequest) error {
	if request.Method == methodInstallPackage {
		// No state verification if the method is to install a package, as the package may
		// not be installed yet.
		return nil
	}

	s, err := d.installer(d.env).State(ctx, request.Package)
	if err != nil {
		return fmt.Errorf("could not get installer state: %w", err)
	}

	c, err := d.installer(d.env).ConfigState(ctx, request.Package)
	if err != nil {
		return fmt.Errorf("could not get installer config state: %w", err)
	}

	installerVersionEqual := request.ExpectedState.InstallerVersion == "" || version.AgentVersion == request.ExpectedState.InstallerVersion
	packageVersionEqual := s.Stable == request.ExpectedState.Stable && s.Experiment == request.ExpectedState.Experiment
	configVersionEqual := c.Stable == request.ExpectedState.StableConfig && c.Experiment == request.ExpectedState.ExperimentConfig

	if installerVersionEqual && (!packageVersionEqual || !configVersionEqual) {
		log.Infof(
			"remote request %s not executed as state does not match: expected %v, got package: %v, config: %v",
			request.ID, request.ExpectedState, s, c,
		)
		setRequestInvalid(ctx)
		d.refreshState(ctx)
		return errStateDoesntMatch
	}

	return nil
}

type requestKey int

var requestStateKey requestKey

// requestState represents the state of a task.
type requestState struct {
	Package   string
	ID        string
	State     pbgo.TaskState
	Err       string
	ErrorCode installerErrors.InstallerErrorCode
}

func newRequestContext(request remoteAPIRequest) (*telemetry.Span, context.Context) {
	ctx := context.WithValue(context.Background(), requestStateKey, &requestState{
		Package: request.Package,
		ID:      request.ID,
		State:   pbgo.TaskState_RUNNING,
	})
	return telemetry.StartSpanFromIDs(ctx, "remote_request", request.TraceID, request.ParentSpanID)
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
		state.Err = err.Error()
		state.ErrorCode = installerErrors.GetCode(err)
	}
}

func (d *daemonImpl) refreshState(ctx context.Context) {
	request, ok := ctx.Value(requestStateKey).(*requestState)
	if ok {
		err := d.taskDB.SetTaskState(*request)
		if err != nil {
			log.Errorf("could not set task state: %v", err)
		}
	}
	state, err := d.installer(d.env).States(ctx)
	if err != nil {
		// TODO: we should report this error through RC in some way
		log.Errorf("could not get installer state: %v", err)
		return
	}
	configState, err := d.installer(d.env).ConfigStates(ctx)
	if err != nil {
		log.Errorf("could not get installer config state: %v", err)
		return
	}
	availableSpace, err := d.installer(d.env).AvailableDiskSpace()
	if err != nil {
		log.Errorf("could not get available size: %v", err)
	}
	tasksState, err := d.taskDB.GetTasksState()
	if err != nil {
		log.Errorf("could not get tasks state: %v", err)
	}
	var packages []*pbgo.PackageState
	for pkg, s := range state {
		p := &pbgo.PackageState{
			Package:                 pkg,
			StableVersion:           s.Stable,
			ExperimentVersion:       s.Experiment,
			StableConfigVersion:     configState[pkg].Stable,
			ExperimentConfigVersion: configState[pkg].Experiment,
		}

		requestState, ok := tasksState[pkg]
		if ok && pkg == requestState.Package {
			var taskErr *pbgo.TaskError
			if requestState.Err != "" {
				taskErr = &pbgo.TaskError{
					Code:    uint64(requestState.ErrorCode),
					Message: requestState.Err,
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
