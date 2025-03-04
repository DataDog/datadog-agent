// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/msi"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	datadogAgent     = "datadog-agent"
	premoteEventName = "Global\\DatadogInstallerPremote"
	stopEventName    = "Global\\DatadogInstallerStop"
)

// PrepareAgent prepares the machine to install the agent
func PrepareAgent(_ context.Context) error {
	return nil // No-op on Windows
}

// SetupAgent installs and starts the agent
// this should no longer be called in as there is no longer a bootstrap process
func SetupAgent(ctx context.Context, args []string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "setup_agent")
	defer func() {
		// Don't log error here, or it will appear twice in the output
		// since installerImpl.Install will also print the error.
		span.Finish(err)
	}()
	// Make sure there are no Agent already installed
	_ = removeAgentIfInstalled(ctx)
	err = installAgentPackage("stable", args, "setup_agent.log")
	return err
}

// GetCurrentAgentMSIProperties returns the MSI path and version of the currently installed Agent
func GetCurrentAgentMSIProperties() (string, string, error) {
	product, err := msi.FindProductCode("Datadog Agent")
	if err != nil {
		return "", "", err
	}

	productVersion, err := msi.GetProductVersion(product.Code)
	if err != nil {
		return "", "", err
	}

	// get MSI path
	msiPath, err := msi.FindProductMSI(product.Code)
	if err != nil {
		return "", "", err
	}

	return msiPath, productVersion, nil

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
	err = startWatchdog(ctx)
	if err != nil {
		log.Errorf("Watchdog failed: %s", err)
		// we failed to start the watchdog, the Agent stopped, or we received a stop-experiment signal
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

func startWatchdog(_ context.Context) error {
	timeout := time.Now().Add(60 * time.Minute)
	premoteEvent, stopEvent, err := createEvents()
	if err != nil {
		return fmt.Errorf("could not create events: %w", err)
	}
	defer closeEvents(premoteEvent, stopEvent)
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
		// TODO do we need to check the status of the agent service?
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
		events, err := windows.WaitForMultipleObjects([]windows.Handle{premoteEvent, stopEvent}, false, 1000)
		if err != nil {
			return fmt.Errorf("could not wait for events: %w", err)
		}
		switch events {
		case windows.WAIT_OBJECT_0:
			// the premote event was signaled
			// this means we are done with the experiment
			// we can return
			return nil
		case windows.WAIT_OBJECT_0 + 1:
			// the stop event was signaled
			// this means we need to restore the stable Agent
			// return an error to signal the caller to restore the stable Agent
			return fmt.Errorf("stop event was signaled")
		}

	}

	return nil

}

// StopAgentExperiment stops the agent experiment, i.e. removes/uninstalls it.
func StopAgentExperiment(_ context.Context) (err error) {
	return setStopEvent()
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(_ context.Context) error {
	return setPremoteEvent()
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
	if os.IsExist(err) {
		rootPath = paths.RootTmpDir
	}
	tempDir, err := os.MkdirTemp(rootPath, "datadog-agent")
	if err != nil {
		return err
	}
	logFile := path.Join(tempDir, logFileName)

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

func createEvents() (windows.Handle, windows.Handle, error) {
	premoteEvent, err := windows.CreateEvent(nil, 1, 0, windows.StringToUTF16Ptr(premoteEventName))
	if err != nil {
		return windows.Handle(0), windows.Handle(0), fmt.Errorf("Failed to create event: %w", err)
	}

	stopEvent, err := windows.CreateEvent(nil, 1, 0, windows.StringToUTF16Ptr(stopEventName))
	if err != nil {
		// close the premoteEvent
		windows.CloseHandle(premoteEvent)
		return windows.Handle(0), windows.Handle(0), fmt.Errorf("Failed to create event: %w", err)
	}

	return premoteEvent, stopEvent, err

}

func closeEvents(premoteEvent, stopEvent windows.Handle) {
	windows.CloseHandle(premoteEvent)
	windows.CloseHandle(stopEvent)
}

func setPremoteEvent() error {
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(premoteEventName))
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

func setStopEvent() error {
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, windows.StringToUTF16Ptr(stopEventName))
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
