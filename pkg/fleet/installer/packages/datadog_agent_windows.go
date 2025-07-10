// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	windowssvc "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/windows"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// datadogAgentPackage is the package for the Datadog Agent
//
// Any hooks that cause the daemon to stop, e.g. uninstall/reinstall the Agent
// or stop/start the Agent services, must run in the background to allow the
// daemon to stop correctly. The Agent package manager for Linux relies on the
// async behavior of systemd to be able to run the hooks synchronously and return
// when they are done. However on Windows we need to perform this work ourselves.
// If this is not followed, then the daemon will fail to report the remote config
// task as DONE, and upon shutdown will hang until the fx timeout is hit.
// Use a custom packag-command to perform this background work.
var datadogAgentPackage = hooks{
	postInstall: postInstallDatadogAgent,
	preRemove:   preRemoveDatadogAgent,

	postStartExperiment:   postStartExperimentDatadogAgent,
	postStopExperiment:    postStopExperimentDatadogAgent,
	postPromoteExperiment: postPromoteExperimentDatadogAgent,

	postStartConfigExperiment:   postStartConfigExperimentDatadogAgent,
	preStopConfigExperiment:     preStopConfigExperimentDatadogAgent,
	postPromoteConfigExperiment: postPromoteConfigExperimentDatadogAgent,
}

const (
	datadogAgent          = "datadog-agent"
	watchdogStopEventName = "Global\\DatadogInstallerStop"
	oldInstallerDir       = "C:\\ProgramData\\Datadog Installer"
)

// postInstallDatadogAgent runs post install scripts for a given package.
func postInstallDatadogAgent(ctx HookContext) error {
	// must get env before uninstalling the Agent since it may read from the registry
	env := getenv()

	// remove the installer if it is installed
	// if nothing is installed this will return without an error
	err := removeInstallerIfInstalled(ctx)
	if err != nil {
		// failed to remove the installer
		return fmt.Errorf("failed to remove installer: %w", err)
	}

	// remove the Agent if it is installed
	// if nothing is installed this will return without an error
	err = removeAgentIfInstalledAndRestartOnFailure(ctx)
	if err != nil {
		// failed to remove the Agent
		return fmt.Errorf("failed to remove Agent: %w", err)
	}

	// install the new stable Agent
	err = installAgentPackage(ctx, env, "stable", ctx.WindowsArgs, "setup_agent.log")
	return err
}

// preRemoveDatadogAgent runs pre remove scripts for a given package.
func preRemoveDatadogAgent(ctx HookContext) (err error) {
	// Don't return an error if the Agent is already not installed.
	// returning an error here will prevent the package from being removed
	// from the local repository.
	if !ctx.Upgrade {
		return removeAgentIfInstalledAndRestartOnFailure(ctx)
	}
	return nil
}

// postStartExperimentDatadogAgent stops the watchdog and launches a new process to start the experiment in the background.
func postStartExperimentDatadogAgent(ctx HookContext) error {
	// open event that signal the end of the experiment
	// this will terminate other running instances of the watchdog
	// this allows for running multiple experiments in sequence
	_ = setWatchdogStopEvent()

	return launchPackageCommandInBackground(ctx.Context, getenv(), "postStartExperimentBackground")
}

// postStartExperimentDatadogAgentBackground uninstalls the Agent, installs the experiment,
// and then stays running to ensure the experiment is running.
//
// Function requirements:
//   - be its own process, not run within the daemon
//   - be run from a copy of the installer, not from the install path,
//     to avoid locking the executable
//
// Rollback notes:
// The Agent package uses an MSI to manage the installation.
// This restricts us to one install present at a time, the previous version
// must always be removed before installing the new version.
// Thus we need a way to rollback to the previous version if installing the
// new version fails, or if the new version fails to start.
// This function/process will stay running for a time after installing the
// new Agent version to ensure the new daemon is running.
//   - If the new daemon is working properly then it will receive "promote"
//     from the backend and will set an event to stop the watchdog.
//   - If the new daemon fails to start, then after a timeout the watchdog will
//     restore the previous version, which should start and then receive
//     "stop experiment" from the backend.
func postStartExperimentDatadogAgentBackground(ctx context.Context) error {
	// must get env before uninstalling the Agent since it may read from the registry
	env := getenv()

	// remove the Agent if it is installed
	// if nothing is installed this will return without an error
	err := removeAgentIfInstalledAndRestartOnFailure(ctx)
	if err != nil {
		return err
	}

	args := getStartExperimentMSIArgs()
	err = installAgentPackage(ctx, env, "experiment", args, "start_agent_experiment.log")
	if err != nil {
		// we failed to install the Agent, we need to restore the stable Agent
		// to leave the system in a consistent state.
		// if the reinstall of the stable fails again we can't do much.
		restoreErr := restoreStableAgentFromExperiment(ctx, env)
		if restoreErr != nil {
			log.Error(restoreErr)
			err = fmt.Errorf("%w, %w", err, restoreErr)
		}
		return err
	}

	// now we start our watchdog to make sure the Agent is running
	// and we can restore the stable Agent if it stops.
	err = startWatchdog(ctx, time.Now().Add(getWatchdogTimeout()))
	if err != nil {
		log.Errorf("Watchdog failed: %s", err)
		// we failed to start the watchdog, the Agent stopped, or we received a timeout
		// we need to restore the stable Agent to leave the system in a consistent state.
		restoreErr := restoreStableAgentFromExperiment(ctx, env)
		if restoreErr != nil {
			log.Error(restoreErr)
			err = fmt.Errorf("%w, %w", err, restoreErr)
		}
		return err
	}

	return nil
}

// postStopExperimentDatadogAgent stops the watchdog and launches a new process to stop the experiment.
func postStopExperimentDatadogAgent(ctx HookContext) (err error) {
	// set watchdog stop to make sure the watchdog stops
	// don't care if it fails cause we will proceed with the stop anyway
	// this will just stop a watchdog that is running
	_ = setWatchdogStopEvent()

	return launchPackageCommandInBackground(ctx.Context, getenv(), "postStopExperimentBackground")
}

// postStopExperimentDatadogAgentBackground uninstalls the Agent and then reinstalls the stable Agent,
//
// Function requirements:
//   - be its own process, not run within the daemon
//   - be run from a copy of the installer, not from the install path,
//     to avoid locking the executable
func postStopExperimentDatadogAgentBackground(ctx context.Context) (err error) {
	// must get env before uninstalling the Agent since it may read from the registry
	env := getenv()

	// remove the Agent
	err = removeAgentIfInstalledAndRestartOnFailure(ctx)
	if err != nil {
		// we failed to remove the Agent
		// we can't do much here
		return fmt.Errorf("failed to remove Agent: %w", err)
	}

	// reinstall the stable Agent
	err = installAgentPackage(ctx, env, "stable", nil, "restore_stable_agent.log")
	if err != nil {
		// we failed to reinstall the stable Agent
		// we can't do much here
		return fmt.Errorf("failed to reinstall stable Agent: %w", err)
	}

	return nil
}

// postPromoteExperimentDatadogAgent runs post promote scripts for a given package.
func postPromoteExperimentDatadogAgent(_ HookContext) error {
	err := setWatchdogStopEvent()
	if err != nil {
		// if we can't set the event it means the watchdog has failed
		// In this case, we were already premoting the experiment
		// so we can return without an error as all we were about to do
		// is stop the watchdog
		log.Errorf("failed to set premote event: %s", err)
	}

	return nil
}

func startWatchdog(_ context.Context, timeout time.Time) error {

	// open events that signal the end of the experiment
	stopEvent, err := createEvent()
	if err != nil {
		return fmt.Errorf("could not create events: %w", err)
	}
	defer windows.CloseHandle(stopEvent)

	// open services we are watching
	// use winutil.OpenSCManager so we can narrow the access permissions
	m, err := winutil.OpenSCManager(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	instService, err := winutil.OpenService(m, "Datadog Installer", windows.SERVICE_QUERY_STATUS|windows.SERVICE_START)
	if err != nil {
		return fmt.Errorf("could not access service: %w", err)
	}
	defer instService.Close()

	dataDogService, err := winutil.OpenService(m, "datadogagent", windows.SERVICE_QUERY_STATUS|windows.SERVICE_START)
	if err != nil {
		return fmt.Errorf("could not access service: %w", err)
	}
	defer dataDogService.Close()

	// Start the services we intend to monitor
	// Relying on the MSI or another tool to start the services before this
	// call can be racy, particularly since the MSI only starts the Agent
	// service, which in turn starts the Installer service sometime later.
	// On success, Start() blocks until the service enters StartPending state.
	_ = instService.Start()
	_ = dataDogService.Start()
	// Ignore errors from starting the services, the following loop will
	// detect if the services are not running and return an error.

	// main watchdog loop
	// Watch the Installer and Agent services and ensure they stay running
	// The Agent MSI starts them initially.
	for time.Now().Before(timeout) {
		// check the Installer service
		status, err := instService.Query()
		if err != nil {
			return fmt.Errorf("could not query service: %w", err)
		}
		if status.State != svc.Running && status.State != svc.StartPending {
			// the service has died
			// we need to restore the stable Agent
			// return an error to signal the caller to restore the stable Agent
			return fmt.Errorf("Datadog Installer is not running")
		}

		// check the Agent service
		status, err = dataDogService.Query()
		if err != nil {
			return fmt.Errorf("could not query service: %w", err)
		}
		if status.State != svc.Running && status.State != svc.StartPending {
			// the service has died
			// we need to restore the stable Agent
			// return an error to signal the caller to restore the stable Agent
			return fmt.Errorf("Datadog Agent is not running")
		}

		// wait for the events to be signaled with a timeout
		events, err := windows.WaitForMultipleObjects([]windows.Handle{stopEvent}, false, 1000)
		if err != nil {
			return fmt.Errorf("could not wait for events: %w", err)
		}
		if events == windows.WAIT_OBJECT_0 {
			// the premote event was signaled
			// this means we are done with the experiment
			// we can return without an error
			return nil
		}

	}

	return fmt.Errorf("watchdog timeout")

}

func installAgentPackage(ctx context.Context, env *env.Env, target string, args []string, logFileName string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "install_agent")
	defer func() { span.Finish(err) }()

	rootPath := ""
	_, err = os.Stat(paths.RootTmpDir)
	// If bootstrap has not been called before, `paths.RootTmpDir` might not exist
	if err == nil {
		// we can use the default tmp dir because it exists
		rootPath = paths.RootTmpDir
	}
	tempDir, err := os.MkdirTemp(rootPath, "datadog-agent")
	if err != nil {
		return err
	}
	logFile := path.Join(tempDir, logFileName)

	// create args
	// need to carry these over as we are uninstalling the agent first
	// and we need to reinstall it with the same configuration
	// and we wipe out our registry keys containing the configuration
	// that the next install would have used
	dataDir := fmt.Sprintf(`APPLICATIONDATADIRECTORY="%s"`, env.MsiParams.ApplicationDataDirectory)
	projectLocation := fmt.Sprintf(`PROJECTLOCATION="%s"`, env.MsiParams.ProjectLocation)

	opts := []msi.MsiexecOption{
		msi.Install(),
		msi.WithMsiFromPackagePath(target, datadogAgent),
		msi.WithLogFile(logFile),
	}
	if env.MsiParams.AgentUserName != "" {
		opts = append(opts, msi.WithDdAgentUserName(env.MsiParams.AgentUserName))
	}
	if env.MsiParams.AgentUserPassword != "" {
		opts = append(opts, msi.WithDdAgentUserPassword(env.MsiParams.AgentUserPassword))
	}
	additionalArgs := []string{"FLEET_INSTALL=1", "SKIP_INSTALL_INFO=1", dataDir, projectLocation}

	// append input args last so they can take precedence
	additionalArgs = append(additionalArgs, args...)
	opts = append(opts, msi.WithAdditionalArgs(additionalArgs))
	cmd, err := msi.Cmd(opts...)

	var output []byte
	if err == nil {
		output, err = cmd.Run()
	}
	if err != nil {
		return fmt.Errorf("failed to install Agent %s: %w\nLog file located at: %s\n%s", target, err, logFile, string(output))
	}
	return nil
}

func removeProductIfInstalled(ctx context.Context, product string) (err error) {
	if msi.IsProductInstalled(product) {
		span, _ := telemetry.StartSpanFromContext(ctx, "remove_agent")
		defer func() {
			if err != nil {
				// removal failed, this should rarely happen.
				// Rollback might have restored the Agent, but we can't be sure.
				log.Errorf("failed to remove agent: %s", err)
			}
			span.Finish(err)
		}()
		err := msi.RemoveProduct(product,
			msi.WithAdditionalArgs([]string{"FLEET_INSTALL=1"}),
		)
		if err != nil {
			return err
		}
	} else {
		log.Debugf("%s not installed", product)
	}
	return nil
}

func removeAgentIfInstalled(ctx context.Context) (err error) {
	// Stop all Datadog Agent services before trying to remove the Agent
	// This helps reduce the chance that we run into the InstallValidate
	// delay issue from msi.dll.
	err = windowssvc.NewWinServiceManager().StopAllAgentServices(ctx)
	if err != nil {
		return fmt.Errorf("failed to stop all Agent services: %w", err)
	}
	return removeProductIfInstalled(ctx, "Datadog Agent")
}

func removeAgentIfInstalledAndRestartOnFailure(ctx context.Context) (err error) {
	err = removeAgentIfInstalled(ctx)
	if err != nil {
		// failed to remove existing Agent, try to restart it if we can.
		// If MSI failed it should rollback to a working state.
		serviceManager := windowssvc.NewWinServiceManager()
		startErr := serviceManager.StartAgentServices(ctx)
		if startErr != nil {
			err = fmt.Errorf("%w, %w", err, startErr)
		}
		return err
	}
	return nil
}

func removeInstallerIfInstalled(ctx context.Context) (err error) {
	if msi.IsProductInstalled("Datadog Installer") {
		err := removeProductIfInstalled(ctx, "Datadog Installer")
		if err != nil {
			return err
		}
		// remove the old installer directory
		// check that owner of oldInstallerDir is admin/system
		if nil == paths.IsDirSecure(oldInstallerDir) {
			err = os.RemoveAll(oldInstallerDir)
			if err != nil {
				return fmt.Errorf("could not remove old installer directory: %w", err)
			}
		} else {
			log.Warnf("old installer directory is not secure, not removing: %s", oldInstallerDir)
		}
	}
	return nil
}

// createEvent returns a new manual reset event that stops the watchdog when set.
// it is expected to be set by the new daemon upon promoteExperiment
//
// https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-createeventw
func createEvent() (windows.Handle, error) {
	return windows.CreateEvent(nil, 1, 0, windows.StringToUTF16Ptr(watchdogStopEventName))
}

func setWatchdogStopEvent() error {
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(watchdogStopEventName))
	if err != nil {
		return fmt.Errorf("failed to open event: %w", err)
	}
	defer windows.CloseHandle(event)

	err = windows.SetEvent(event)
	if err != nil {
		return fmt.Errorf("failed to set event: %w", err)
	}
	return nil
}

// getWatchdogTimeout returns the duration the watchdog will run before stopping the experiment
// and restoring the stable Agent.
//
// Default is 60 minutes.
//
// The timeout can be configured by setting the registry key to the desired timeout in minutes:
// `HKEY_LOCAL_MACHINE\SOFTWARE\Datadog\Datadog Agent\WatchdogTimeout`
func getWatchdogTimeout() time.Duration {
	defaultTimeout := 60 * time.Minute

	// open the registry key
	keyname := "SOFTWARE\\Datadog\\Datadog Agent"
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root not found, using default")
		return defaultTimeout
	}
	defer k.Close()
	val, _, err := k.GetIntegerValue("WatchdogTimeout")
	if err != nil {
		log.Warnf("Windows installation key watchdogTimeout not found, using default")
		return defaultTimeout
	}
	return time.Duration(val) * time.Minute
}

// getAgentUserNameFromRegistry returns the user name for the Agent, stored in the registry by the Agent MSI
func getAgentUserNameFromRegistry() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, "SOFTWARE\\Datadog\\Datadog Agent", registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()

	user, _, err := k.GetStringValue("installedUser")
	if err != nil {
		return "", fmt.Errorf("could not read installedUser in registry: %w", err)
	}

	domain, _, err := k.GetStringValue("installedDomain")
	if err != nil {
		return "", fmt.Errorf("could not read installedDomain in registry: %w", err)
	}

	if domain != "" {
		user = domain + `\` + user
	}

	return user, nil
}

// getenv returns an Env struct with values from the environment, supplemented by values from the registry.
//
// See also env.FromEnv()
//
// This function prefers values from the environment, falling back to the registry if not set, for values:
//   - Agent user name
//   - Project location
//   - Application data directory
//
// This accomplishes the following:
//   - ensures setup carries over settings from previous installs (i.e. before remote updates)
//   - ensures subcommands provide the correct options even if the MSI removes the registry keys (like during rollback)
func getenv() *env.Env {
	env := env.FromEnv()

	// fallback to registry for agent user
	if env.MsiParams.AgentUserName == "" {
		user, err := getAgentUserNameFromRegistry()
		if err != nil {
			log.Warnf("Could not read Agent user from registry: %v", err)
		} else {
			env.MsiParams.AgentUserName = user
		}
	}

	// fallback to registry for custom paths
	if env.MsiParams.ProjectLocation == "" {
		env.MsiParams.ProjectLocation = paths.DatadogProgramFilesDir
	}
	if env.MsiParams.ApplicationDataDirectory == "" {
		env.MsiParams.ApplicationDataDirectory = paths.DatadogDataDir
	}

	return env
}

func newInstallerExec(env *env.Env) (*exec.InstallerExec, error) {
	installerBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("could not get installer executable path: %w", err)
	}
	installerBin, err = filepath.EvalSymlinks(installerBin)
	if err != nil {
		return nil, fmt.Errorf("could not get resolve installer executable path: %w", err)
	}
	installer := exec.NewInstallerExec(env, installerBin)
	return installer, nil
}

// restoreStableAgentFromExperiment restores the stable Agent using the remove-experiment command.
//
// call remove-experiment to:
//   - remove current version and reinstall stable version
//   - update repository state / remove experiment link
//
// The updated repository state will cause the stable daemon to skip the stop-experiment
// operation received from the backend, which avoids reinstalling the stable Agent again.
func restoreStableAgentFromExperiment(ctx context.Context, env *env.Env) error {
	installer, err := newInstallerExec(env)
	if err != nil {
		return fmt.Errorf("failed to create installer exec: %w", err)
	}
	err = installer.RemoveExperiment(ctx, datadogAgent)
	if err != nil {
		return fmt.Errorf("failed to restore stable Agent: %w", err)
	}

	return nil
}

// getStartExperimentMSIArgs returns additional MSI arguments to be passed during experiment installation.
//
// This is primarily used for testing purposes to inject custom MSI arguments during experiment installation,
// for example, to add WIXFAILWHENDEFERRED=1 to test MSI rollback.
// The arguments are read from the registry key "HKLM\SOFTWARE\Datadog\Datadog Agent\ExperimentMSIArgs".
func getStartExperimentMSIArgs() []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Datadog\Datadog Agent`,
		registry.ALL_ACCESS)
	if err != nil {
		return []string{}
	}
	defer k.Close()

	args, _, err := k.GetStringsValue("StartExperimentMSIArgs")
	if err != nil {
		return []string{}
	}

	return args
}

// setFleetPoliciesDir sets the fleet_policies_dir registry value to the given path.
//
// On Agent start, the config package copies this registry key to the Agent config value
// of the same name using Config.AddOverrideFunc.
func setFleetPoliciesDir(path string) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		"SOFTWARE\\Datadog\\Datadog Agent",
		registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer k.Close()

	err = k.SetStringValue("fleet_policies_dir", path)
	if err != nil {
		return fmt.Errorf("failed to set fleet_policies_dir registry key: %w", err)
	}
	return nil
}

// postStartConfigExperimentDatadogAgent stops the watchdog, sets the fleet_policies_dir to experiment,
// and launches a new process to start the experiment in the background.
func postStartConfigExperimentDatadogAgent(ctx HookContext) error {
	// open event that signal the end of the experiment
	// this will terminate other running instances of the watchdog
	// this allows for running multiple experiments in sequence
	_ = setWatchdogStopEvent()

	// Set the registry key to point to the experiment config
	experimentPath := filepath.Join(paths.ConfigsPath, "datadog-agent", "experiment")
	err := setFleetPoliciesDir(experimentPath)
	if err != nil {
		return err
	}

	return launchPackageCommandInBackground(ctx.Context, getenv(), "postStartConfigExperimentBackground")
}

// postStartConfigExperimentDatadogAgentBackground restarts the Agent services and then
// stays running to ensure the experiment is running.
//
// Function requirements:
//   - be its own process, not run within the daemon
//
// Rollback notes:
// The config experiment uses a watchdog to monitor the Agent service.
// If the service fails to start or stops running, the watchdog will restore
// the stable config using the remove-config-experiment command.
// This ensures the system remains in a consistent state even if the experiment
// config causes issues.
//   - If the new config is working properly then it will receive "promote"
//     from the backend and will set an event to stop the watchdog.
//   - If the new config fails to start the Agent, then after a timeout the
//     watchdog will restore the stable config.
func postStartConfigExperimentDatadogAgentBackground(ctx context.Context) error {
	// Start the agent service to pick up the new config
	err := windowssvc.NewWinServiceManager().RestartAgentServices(ctx)
	if err != nil {
		// Agent failed to start, restore stable config
		restoreErr := restoreStableConfigFromExperiment(ctx)
		if restoreErr != nil {
			log.Error(restoreErr)
			err = fmt.Errorf("%w, %w", err, restoreErr)
		}
		return fmt.Errorf("failed to start agent service: %w", err)
	}

	// Start watchdog to monitor the agent service
	timeout := getWatchdogTimeout()
	err = startWatchdog(ctx, time.Now().Add(timeout))
	if err != nil {
		log.Errorf("Config watchdog failed: %s", err)
		// If watchdog fails, restore stable config
		restoreErr := restoreStableConfigFromExperiment(ctx)
		if restoreErr != nil {
			log.Error(restoreErr)
			err = fmt.Errorf("%w, %w", err, restoreErr)
		}
		return err
	}

	return nil
}

// restoreStableConfigFromExperiment restores the stable config using the remove-config-experiment command.
//
// call remove-config-experiment to:
//   - restore stable config
//   - update repository state / remove experiment link
//
// The updated repository state will cause the stable daemon to skip the stop-experiment
// operation received from the backend, which avoids restarting the services again.
func restoreStableConfigFromExperiment(ctx context.Context) error {
	env := getenv()
	installer, err := newInstallerExec(env)
	if err != nil {
		return fmt.Errorf("failed to create installer exec: %w", err)
	}
	err = installer.RemoveConfigExperiment(ctx, datadogAgent)
	if err != nil {
		return fmt.Errorf("failed to restore stable config: %w", err)
	}

	return nil
}

// preStopConfigExperimentDatadogAgent stops the watchdog, sets the fleet_policies_dir to stable,
// and launches a new process to stop the experiment in the background.
func preStopConfigExperimentDatadogAgent(ctx HookContext) error {
	// set watchdog stop to make sure the watchdog stops
	// don't care if it fails cause we will proceed with the stop anyway
	// this will just stop a watchdog that is running
	_ = setWatchdogStopEvent()

	// Set the registry key to point to the previous stable config
	stablePath := filepath.Join(paths.ConfigsPath, "datadog-agent", "stable")
	err := setFleetPoliciesDir(stablePath)
	if err != nil {
		return err
	}

	return launchPackageCommandInBackground(ctx.Context, getenv(), "preStopConfigExperimentBackground")
}

// preStopConfigExperimentDatadogAgentBackground restarts the Agent services.
func preStopConfigExperimentDatadogAgentBackground(ctx context.Context) error {
	// Start the agent service to pick up the stable config
	err := windowssvc.NewWinServiceManager().RestartAgentServices(ctx)
	if err != nil {
		return fmt.Errorf("failed to start agent service: %w", err)
	}
	return nil
}

// postPromoteConfigExperimentDatadogAgent stops the watchdog, sets the fleet_policies_dir to stable,
// and launches a new process to promote the experiment in the background.
func postPromoteConfigExperimentDatadogAgent(ctx HookContext) error {
	err := setWatchdogStopEvent()
	if err != nil {
		// if we can't set the event it means the watchdog has failed
		// In this case, we were already promoting the experiment
		// so we can continue without error
		log.Errorf("failed to set premote event: %s", err)
	}

	// Set the registry key to point to the stable config (which now contains the promoted experiment)
	stablePath := filepath.Join(paths.ConfigsPath, "datadog-agent", "stable")
	err = setFleetPoliciesDir(stablePath)
	if err != nil {
		return err
	}

	return launchPackageCommandInBackground(ctx.Context, getenv(), "postPromoteConfigExperimentBackground")
}

// postPromoteConfigExperimentDatadogAgentBackground restarts the Agent services.
func postPromoteConfigExperimentDatadogAgentBackground(ctx context.Context) error {
	// Start the agent service to pick up the promoted config
	err := windowssvc.NewWinServiceManager().RestartAgentServices(ctx)
	if err != nil {
		return fmt.Errorf("failed to start agent service: %w", err)
	}
	return nil
}

// runDatadogAgentPackageCommand maps the package specific command names to their corresponding functions.
func runDatadogAgentPackageCommand(ctx context.Context, command string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, command)
	defer func() { span.Finish(err) }()

	switch command {
	case "postStartExperimentBackground":
		return postStartExperimentDatadogAgentBackground(ctx)
	case "postStopExperimentBackground":
		return postStopExperimentDatadogAgentBackground(ctx)
	case "postStartConfigExperimentBackground":
		return postStartConfigExperimentDatadogAgentBackground(ctx)
	case "preStopConfigExperimentBackground":
		return preStopConfigExperimentDatadogAgentBackground(ctx)
	case "postPromoteConfigExperimentBackground":
		return postPromoteConfigExperimentDatadogAgentBackground(ctx)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

// launchPackageCommandInBackground launches a package command in the background using the installer.
func launchPackageCommandInBackground(ctx context.Context, env *env.Env, command string) error {
	installer, err := newInstallerExec(env)
	if err != nil {
		return fmt.Errorf("failed to create installer exec: %w", err)
	}

	err = installer.StartPackageCommandDetached(ctx, datadogAgent, command)
	if err != nil {
		return fmt.Errorf("failed to start background process: %w", err)
	}

	return nil
}
