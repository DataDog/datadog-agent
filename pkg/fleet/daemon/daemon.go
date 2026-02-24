// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package daemon implements the fleet long running daemon.
package daemon

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
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

	"golang.org/x/crypto/nacl/box"

	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/bootstrap"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
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
	// disableClientIDCheck is the magic string to disable the client ID check.
	disableClientIDCheck = "disable-client-id-check"
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
	StartConfigExperiment(ctx context.Context, pkg string, operations config.Operations, encryptedSecrets map[string]string) error
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

	ctx    context.Context
	cancel context.CancelFunc

	env             *env.Env
	installer       func(*env.Env) installer.Installer
	rc              *remoteConfig
	catalog         catalog
	catalogOverride catalog
	configs         map[string]installerConfig
	configsOverride map[string]installerConfig
	requests        chan remoteAPIRequest
	requestsWG      sync.WaitGroup
	goroutineWG     sync.WaitGroup
	taskDB          *taskDB
	clientID        string
	refreshInterval time.Duration
	gcInterval      time.Duration

	secretsPubKey, secretsPrivKey *[32]byte
}

func newInstaller(installerBin string) func(env *env.Env) installer.Installer {
	return func(env *env.Env) installer.Installer {
		return exec.NewInstallerExec(env, installerBin)
	}
}

// NewDaemon returns a new daemon.
func NewDaemon(hostname string, rcFetcher client.ConfigFetcher, config agentconfig.Reader) (Daemon, error) {
	installerBin, err := exec.GetExecutable()
	if err != nil {
		return nil, fmt.Errorf("could not get installer executable path: %w", err)
	}
	installerBin, err = filepath.EvalSymlinks(installerBin)
	if err != nil {
		return nil, fmt.Errorf("could not get resolve installer executable path: %w", err)
	}
	if runtime.GOOS != "windows" {
		installerBin = filepath.Join(filepath.Dir(installerBin), "..", "..", "embedded", "bin", "installer")
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
	configID := config.GetString("config_id")
	if configID == "" {
		configID = "empty"
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
		ConfigID:             configID,
	}
	installer := newInstaller(installerBin)
	refreshInterval := config.GetDuration("installer.refresh_interval")
	gcInterval := config.GetDuration("installer.gc_interval")

	secretsPubKey, secretsPrivKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("could not generate box key: %w", err)
	}

	return newDaemon(rc, installer, env, taskDB, refreshInterval, gcInterval, secretsPubKey, secretsPrivKey), nil
}

func newDaemon(rc *remoteConfig, installer func(env *env.Env) installer.Installer, env *env.Env, taskDB *taskDB, refreshInterval time.Duration, gcInterval time.Duration, secretsPubKey, secretsPrivKey *[32]byte) *daemonImpl {
	ctx, cancel := context.WithCancel(context.Background())
	i := &daemonImpl{
		env:             env,
		clientID:        rc.client.GetClientID(),
		rc:              rc,
		installer:       installer,
		requests:        make(chan remoteAPIRequest, 32),
		catalog:         catalog{},
		catalogOverride: catalog{},
		configs:         make(map[string]installerConfig),
		configsOverride: make(map[string]installerConfig),
		stopChan:        make(chan struct{}),
		taskDB:          taskDB,
		refreshInterval: refreshInterval,
		gcInterval:      gcInterval,
		secretsPubKey:   secretsPubKey,
		secretsPrivKey:  secretsPrivKey,
		ctx:             ctx,
		cancel:          cancel,
	}
	return i
}

// GetState returns the state.
func (d *daemonImpl) GetState(ctx context.Context) (map[string]PackageState, error) {
	d.m.Lock()
	defer d.m.Unlock()

	configAndPackageStates, err := d.installer(d.env).ConfigAndPackageStates(ctx)
	if err != nil {
		return nil, err
	}

	res := make(map[string]PackageState)
	for pkg := range configAndPackageStates.States {
		res[pkg] = PackageState{
			Version: configAndPackageStates.States[pkg],
			Config:  configAndPackageStates.ConfigStates[pkg],
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

func (d *daemonImpl) getConfig(version string) (installerConfig, error) {
	configs := d.configs
	if len(d.configsOverride) > 0 {
		configs = d.configsOverride
	}

	config, ok := configs[version]
	if !ok {
		return installerConfig{}, fmt.Errorf("config version %s not found in available configs", version)
	}
	return config, nil
}

// decryptSecrets decrypts the encrypted secrets and returns them as a map.
// It does NOT replace them in the operations - that will be done by the installer binary.
// This is to avoid leaking the secrets in argv/envp.
func (d *daemonImpl) decryptSecrets(operations config.Operations, encryptedSecrets map[string]string) (map[string]string, error) {
	decryptedSecrets := make(map[string]string)

	for key, encoded := range encryptedSecrets {
		// 1. Check if any file operation in the config contains SEC[key]
		found := false
		for _, operation := range operations.FileOperations {
			if strings.Contains(string(operation.Patch), fmt.Sprintf("SEC[%s]", key)) {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		// 2. Decode the base64 encoded secret
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("could not decode secret %s: %w", key, err)
		}

		// 3. Decrypt the secret
		decrypted, ok := box.OpenAnonymous(nil, raw, d.secretsPubKey, d.secretsPrivKey)
		if !ok {
			return nil, fmt.Errorf("could not decrypt secret %s", key)
		}

		// 4. Store the decrypted secret
		decryptedSecrets[key] = string(decrypted)
	}

	return decryptedSecrets, nil
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

	// If FIPS is enabled, don't start the daemon
	fipsEnabled, err := fips.Enabled()
	if err != nil {
		log.Warnf("Could not determine FIPS status, exiting: %v", err)
		return nil
	}
	if fipsEnabled {
		log.Info("FIPS mode is enabled, fleet daemon will not start")
		return nil
	}

	d.goroutineWG.Go(func() {
		// Run the initial state refresh inside the goroutine so that FX init
		// completes quickly even when packages.db is locked by a concurrent
		// installer process.  This ensures signal handlers are registered
		// before any blocking subprocess is spawned, preventing orphaned
		// subprocesses that would linger in the systemd cgroup.
		// refreshState only reads external state (installer subprocess, taskDB,
		// immutable daemon fields) so it is safe to call without d.m.
		d.refreshState(d.ctx)

		// Start the RC client only after the initial state refresh so that the
		// first RC payload sent to the backend contains the actual package state
		// instead of an empty state.  The mutex guards against a race with
		// rc.Close() in Stop().
		d.m.Lock()
		if d.ctx.Err() == nil {
			d.rc.Start(d.handleConfigsUpdate, d.handleCatalogUpdate, d.scheduleRemoteAPIRequest)
		}
		d.m.Unlock()

		gcTicker := time.NewTicker(d.gcInterval)
		defer gcTicker.Stop()
		refreshStateTicker := time.NewTicker(d.refreshInterval)
		defer refreshStateTicker.Stop()
		for {
			select {
			case <-gcTicker.C:
				d.m.Lock()
				err := d.installer(d.env).GarbageCollect(d.ctx)
				d.m.Unlock()
				if err != nil {
					log.Errorf("Daemon: could not run GC: %v", err)
				}
			case <-refreshStateTicker.C:
				d.m.Lock()
				d.refreshState(d.ctx)
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
	})
	return nil
}

// Stop stops the garbage collector.
func (d *daemonImpl) Stop(_ context.Context) error {
	// Cancel the daemon context before acquiring the mutex so that any in-flight
	// child processes spawned by refreshState() are killed immediately.
	d.cancel()

	d.m.Lock()

	// Always close the remote config client as it was initialized in NewDaemon; avoid unknown side effects in the RC client
	d.rc.Close()

	// If remote updates are disabled, we don't need to stop the updater daemon background goroutine as it was never started, we return early
	if !d.env.RemoteUpdates {
		err := d.taskDB.Close()
		d.m.Unlock()
		return err
	}

	// Same, if FIPS is enabled, the updater daemon background goroutine was never started, we return early
	fipsEnabled, err := fips.Enabled()
	if err != nil {
		log.Warnf("Could not determine FIPS status: %v", err)
	}
	if fipsEnabled {
		err = d.taskDB.Close()
		d.m.Unlock()
		return err
	}

	// Stop the background goroutine
	close(d.stopChan)
	// Release the lock before waiting so that the background goroutine can still
	// acquire d.m if needed (e.g. handleRemoteAPIRequest, periodic refreshState).
	d.m.Unlock()

	// Wait for the background goroutine to exit so that any in-flight subprocess
	// (e.g. get-states or install-package blocked on packages.db) is waited on
	// by exec.Cmd.Wait().  This keeps the daemon process alive long enough for
	// WaitDelay (15s) to fire and SIGKILL the subprocess, preventing it from
	// being orphaned in the systemd cgroup and causing a 90s stop timeout.
	d.goroutineWG.Wait()
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
func (d *daemonImpl) StartConfigExperiment(ctx context.Context, pkg string, operations config.Operations, encryptedSecrets map[string]string) error {
	d.m.Lock()
	defer d.m.Unlock()
	return d.startConfigExperiment(ctx, pkg, operations, encryptedSecrets)
}

func (d *daemonImpl) startConfigExperiment(ctx context.Context, pkg string, operations config.Operations, encryptedSecrets map[string]string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "start_config_experiment")
	defer func() { span.Finish(err) }()
	d.refreshState(ctx)
	defer d.refreshState(ctx)

	log.Infof("Daemon: Starting config experiment for package %s (deployment id: %s)", pkg, operations.DeploymentID)

	// Decrypt secrets but don't replace them in operations yet
	decryptedSecrets, err := d.decryptSecrets(operations, encryptedSecrets)
	if err != nil {
		return fmt.Errorf("could not decrypt secrets: %w", err)
	}

	// Pass operations with placeholders and decrypted secrets to installer
	// The installer will do the replacement
	err = d.installer(d.env).InstallConfigExperiment(ctx, pkg, operations, decryptedSecrets)
	if err != nil {
		return fmt.Errorf("could not start config experiment: %w", err)
	}
	log.Infof("Daemon: Successfully started config experiment for package %s (deployment id: %s)", pkg, operations.DeploymentID)
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
	select {
	case d.requests <- request:
	case <-d.stopChan:
		d.requestsWG.Done()
	}
	return nil
}

func (d *daemonImpl) handleRemoteAPIRequest(request remoteAPIRequest) (err error) {
	d.m.Lock()
	defer d.m.Unlock()
	defer d.requestsWG.Done()
	parentSpan, ctx := newRequestContext(d.ctx, request)
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
		c, err := d.getConfig(params.Version)
		if err != nil {
			return fmt.Errorf("could not get config: %w", err)
		}
		var ops config.Operations
		ops.DeploymentID = c.ID
		for _, operation := range c.FileOperations {
			ops.FileOperations = append(ops.FileOperations, config.FileOperation{
				FileOperationType: config.FileOperationType(operation.FileOperationType),
				FilePath:          operation.FilePath,
				Patch:             operation.Patch,
			})
		}
		encryptedSecrets := make(map[string]string)
		for _, secret := range params.EncryptedSecrets {
			encryptedSecrets[secret.Key] = secret.EncryptedValue
		}
		return d.startConfigExperiment(ctx, request.Package, ops, encryptedSecrets)

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
	clientIDEqual := d.clientID == request.ExpectedState.ClientID || request.ExpectedState.ClientID == disableClientIDCheck
	if installerVersionEqual && (!packageVersionEqual || !configVersionEqual || !clientIDEqual) {
		log.Infof(
			"remote request %s not executed as state does not match: expected %v, got package: %v, config: %v, client id: %s",
			request.ID, request.ExpectedState, s, c, d.clientID,
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

func newRequestContext(baseCtx context.Context, request remoteAPIRequest) (*telemetry.Span, context.Context) {
	ctx := context.WithValue(baseCtx, requestStateKey, &requestState{
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

	configAndPackageStates, err := d.installer(d.env).ConfigAndPackageStates(ctx)
	if err != nil {
		// TODO: we should report this error through RC in some way
		log.Errorf("could not get installer config and package states: %v", err)
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
	runningVersions := map[string]string{
		"datadog-agent": version.AgentPackageVersion,
	}
	runningConfigVersions := map[string]string{
		"datadog-agent": d.env.ConfigID,
	}
	var packages []*pbgo.PackageState
	for pkg, s := range configAndPackageStates.States {
		p := &pbgo.PackageState{
			Package:                 pkg,
			StableVersion:           s.Stable,
			ExperimentVersion:       s.Experiment,
			StableConfigVersion:     configAndPackageStates.ConfigStates[pkg].Stable,
			ExperimentConfigVersion: configAndPackageStates.ConfigStates[pkg].Experiment,
			RunningVersion:          runningVersions[pkg],
			RunningConfigVersion:    runningConfigVersions[pkg],
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
		SecretsPubKey:      base64.StdEncoding.EncodeToString(d.secretsPubKey[:]),
		Packages:           packages,
		AvailableDiskSpace: availableSpace,
	})
}
