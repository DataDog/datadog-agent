// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	datadogAgent          = "datadog-agent"
	watchdogStopEventName = "Global\\DatadogInstallerStop"
)

// PrepareAgent prepares the machine to install the agent
func PrepareAgent(_ context.Context) error {
	return nil // No-op on Windows
}

// SetupAgent installs and starts the agent
func SetupAgent(_ context.Context, _ []string) (err error) {
	return nil
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "start_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to start agent experiment: %s", err)
		}
		span.Finish(err)
	}()

	// open event that signal the end of the experiment
	// this will terminate other running instances of the watchdog
	// this allows for running multiple experiments in sequence
	_ = setWatchdogStopEvent()

	timeout := getWatchdogTimeout()

	err = removeAgentIfInstalled(ctx)
	if err != nil {
		return err
	}

	err = installAgentPackage("experiment", nil, "start_agent_experiment.log")
	if err != nil {
		// we failed to install the Agent, we need to restore the stable Agent
		// to leave the system in a consistent state.
		// if the reinstall of the sable fails again we can't do much.
		_ = installAgentPackage("stable", nil, "restore_stable_agent.log")
		return err
	}

	// now we start our watchdog to make sure the Agent is running
	// and we can restore the stable Agent if it stops.
	err = startWatchdog(ctx, time.Now().Add(time.Duration(timeout)*time.Minute))
	if err != nil {
		log.Errorf("Watchdog failed: %s", err)
		// we failed to start the watchdog, the Agent stopped, or we received a timeout
		// we need to restore the stable Agent
		// to leave the system in a consistent state.
		// remove the experiment Agent
		err = removeAgentIfInstalled(ctx)
		if err != nil {
			// we failed to remove the experiment Agent
			// we can't do much here
			log.Errorf("Failed to remove experiment Agent: %s", err)
			return fmt.Errorf("Failed to remove experiment Agent: %w", err)
		}
		// reinstall the stable Agent
		_ = installAgentPackage("stable", nil, "restore_stable_agent.log")
		return err
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
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	instService, err := m.OpenService("Datadog Installer")
	if err != nil {
		return fmt.Errorf("could not access service: %w", err)
	}
	defer instService.Close()

	dataDogService, err := m.OpenService("datadogagent")
	if err != nil {
		return fmt.Errorf("could not access service: %w", err)
	}
	defer dataDogService.Close()

	// main watchdog loop
	for time.Now().Before(timeout) {
		// check the Installer service
		status, err := instService.Query()
		if err != nil {
			return fmt.Errorf("could not query service: %w", err)
		}
		if status.State != svc.Running {
			// the service has died
			// we need to restore the stable Agent
			// return an error to signal the caller to restore the stable Agent
			return fmt.Errorf("Agent is not running")
		}

		// check the Agent service
		status, err = dataDogService.Query()
		if err != nil {
			return fmt.Errorf("could not query service: %w", err)
		}
		if status.State != svc.Running {
			// the service has died
			// we need to restore the stable Agent
			// return an error to signal the caller to restore the stable Agent
			return fmt.Errorf("Agent is not running")
		}

		// wait for the events to be singaled with a timeout
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

	return fmt.Errorf("Watchdog timeout")

}

// StopAgentExperiment stops the agent experiment, i.e. removes/uninstalls it.
func StopAgentExperiment(ctx context.Context) (err error) {
	// set watchdog stop to make sure the watchdog stops
	// don't care if it fails cause we will proceed with the stop anyway
	// this will just stop a watchdog that is running
	_ = setWatchdogStopEvent()

	// remove the Agent
	err = removeAgentIfInstalled(ctx)
	if err != nil {
		// we failed to remove the Agent
		// we can't do much here
		return fmt.Errorf("Failed to remove Agent: %w", err)
	}

	// reinstall the stable Agent
	err = installAgentPackage("stable", nil, "restore_stable_agent.log")
	if err != nil {
		// we failed to reinstall the stable Agent
		// we can't do much here
		return fmt.Errorf("Failed to reinstall stable Agent: %w", err)
	}

	return nil
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(_ context.Context) error {
	err := setWatchdogStopEvent()
	if err != nil {
		// if we can't set the event it means the watchdog has failed
		// In this case, we were already premoting the experiment
		// so we can return without an error as all we were about to do
		// is stop the watchdog
		log.Errorf("Failed to set premote event: %s", err)
	}

	return nil
}

// RemoveAgent stops and removes the agent
func RemoveAgent(ctx context.Context) (err error) {
	// Don't return an error if the Agent is already not installed.
	// returning an error here will prevent the package from being removed
	// from the local repository.
	return removeAgentIfInstalled(ctx)
}

func installAgentPackage(target string, args []string, logFileName string) error {

	rootPath := ""
	_, err := os.Stat(paths.RootTmpDir)
	// If bootstrap has not been called before, `paths.RootTmpDir` might not exist
	if errors.Is(err, fs.ErrNotExist) {
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
	dataDir := fmt.Sprintf(`APPLICATIONDATADIRECTORY="%s"`, paths.DatadogDataDir)
	projectLocation := fmt.Sprintf(`PROJECTLOCATION="%s"`, paths.DatadogProgramFilesDir)

	args = append(args, "FLEET_INSTALL=1", dataDir, projectLocation)

	cmd, err := msi.Cmd(
		msi.Install(),
		msi.WithMsiFromPackagePath(target, datadogAgent),
		msi.WithAdditionalArgs(args),
		msi.WithLogFile(logFile),
	)
	var output []byte
	if err == nil {
		output, err = cmd.Run()
	}
	if err != nil {
		return fmt.Errorf("failed to install Agent %s: %w\nLog file located at: %s\n%s", target, err, logFile, string(output))
	}
	return nil
}

func removeAgentIfInstalled(ctx context.Context) (err error) {
	if msi.IsProductInstalled("Datadog Agent") {
		span, _ := telemetry.StartSpanFromContext(ctx, "remove_agent")
		defer func() {
			if err != nil {
				// removal failed, this should rarely happen.
				// Rollback might have restored the Agent, but we can't be sure.
				log.Errorf("Failed to remove agent: %s", err)
			}
			span.Finish(err)
		}()
		err := msi.RemoveProduct("Datadog Agent")
		if err != nil {
			return err
		}
	} else {
		log.Debugf("Agent not installed")
	}
	return nil
}

func createEvent() (windows.Handle, error) {
	return windows.CreateEvent(nil, 1, 0, windows.StringToUTF16Ptr(watchdogStopEventName))
}

func setWatchdogStopEvent() error {
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(watchdogStopEventName))
	if err != nil {
		return fmt.Errorf("Failed to open event: %w", err)
	}
	defer windows.CloseHandle(event)

	err = windows.SetEvent(event)
	if err != nil {
		return fmt.Errorf("Failed to set event: %w", err)
	}
	return nil
}

func getWatchdogTimeout() uint64 {
	// get optional registry key for watchdog timeout
	// if not set default to 60 minutes
	// this is the time the watchdog will run before stopping the experiment
	// and restoring the stable Agent
	var timeout uint64 = 60

	// open the registry key
	keyname := "SOFTWARE\\Datadog\\Datadog Agent"
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root not found, using timeout")
		return timeout
	}
	defer k.Close()
	val, _, err := k.GetIntegerValue("WatchdogTimeout")
	if err != nil {
		log.Warnf("Windows installation key wathdogTimeout not found, using default")
		return timeout
	}
	return val
}
