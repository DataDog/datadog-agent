// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// datadogAgentPackage is the package for the Datadog Agent
var datadogAgentPackage = hooks{
	postInstall:           postInstallDatadogAgent,
	preRemove:             preRemoveDatadogAgent,
	postStartExperiment:   postStartExperimentDatadogAgent,
	postStopExperiment:    postStopExperimentDatadogAgent,
	postPromoteExperiment: postPromoteExperimentDatadogAgent,
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
	err = removeAgentIfInstalled(ctx)
	if err != nil {
		// failed to remove the Agent
		return fmt.Errorf("failed to remove Agent: %w", err)
	}

	// install the new stable Agent
	err = installAgentPackage(env, "stable", ctx.WindowsArgs, "setup_agent.log")
	return err
}

// preRemoveDatadogAgent runs pre remove scripts for a given package.
func preRemoveDatadogAgent(ctx HookContext) (err error) {
	// Don't return an error if the Agent is already not installed.
	// returning an error here will prevent the package from being removed
	// from the local repository.
	if !ctx.Upgrade {
		return removeAgentIfInstalled(ctx)
	}
	return nil
}

// postStartExperimentDatadogAgent runs post start scripts for a given package.
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
func postStartExperimentDatadogAgent(ctx HookContext) error {
	// must get env before uninstalling the Agent since it may read from the registry
	env := getenv()

	// open event that signal the end of the experiment
	// this will terminate other running instances of the watchdog
	// this allows for running multiple experiments in sequence
	_ = setWatchdogStopEvent()

	timeout := getWatchdogTimeout()

	err := removeAgentIfInstalled(ctx)
	if err != nil {
		return err
	}

	args := getStartExperimentMSIArgs()
	err = installAgentPackage(env, "experiment", args, "start_agent_experiment.log")
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
	err = startWatchdog(ctx, time.Now().Add(timeout))
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

// postStopExperimentDatadogAgent runs post stop scripts for a given package.
//
// Function requirements:
//   - be its own process, not run within the daemon
//   - be run from a copy of the installer, not from the install path,
//     to avoid locking the executable
func postStopExperimentDatadogAgent(ctx HookContext) (err error) {
	// set watchdog stop to make sure the watchdog stops
	// don't care if it fails cause we will proceed with the stop anyway
	// this will just stop a watchdog that is running
	_ = setWatchdogStopEvent()

	// must get env before uninstalling the Agent since it may read from the registry
	env := getenv()

	// remove the Agent
	err = removeAgentIfInstalled(ctx)
	if err != nil {
		// we failed to remove the Agent
		// we can't do much here
		return fmt.Errorf("failed to remove Agent: %w", err)
	}

	// reinstall the stable Agent
	err = installAgentPackage(env, "stable", nil, "restore_stable_agent.log")
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

	instService, err := winutil.OpenService(m, "Datadog Installer", windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return fmt.Errorf("could not access service: %w", err)
	}
	defer instService.Close()

	dataDogService, err := winutil.OpenService(m, "datadogagent", windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return fmt.Errorf("could not access service: %w", err)
	}
	defer dataDogService.Close()

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

func installAgentPackage(env *env.Env, target string, args []string, logFileName string) error {

	rootPath := ""
	_, err := os.Stat(paths.RootTmpDir)
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
	additionalArgs := []string{"FLEET_INSTALL=1", dataDir, projectLocation}

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
	// Stop the Datadog Agent services before trying to remove it
	// As datadogagent will shutdown the installer service when it stops
	// we do not need to stop the installer service
	log.Infof("stopping the datadogagent service")
	err = winutil.StopService("datadogagent")
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			log.Infof("the datadogagent service is not present on this machine, skipping stop action")
		} else {
			// Only fail if the service exists
			return fmt.Errorf("failed to stop the datadogagent service: %w", err)
		}
	}
	return removeProductIfInstalled(ctx, "Datadog Agent")
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
func restoreStableAgentFromExperiment(ctx HookContext, env *env.Env) error {
	installer, err := newInstallerExec(env)
	if err != nil {
		return fmt.Errorf("failed to create installer exec: %w", err)
	}
	err = installer.RemoveExperiment(ctx, ctx.Package)
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
