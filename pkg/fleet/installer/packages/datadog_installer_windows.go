// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package packages contains the install/upgrades/uninstall logic for packages
package packages

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/msi"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
)

var stableRecoveryOptions = []mgr.RecoveryAction{
	{
		Type:  mgr.ServiceRestart,
		Delay: 1 * time.Minute,
	},
	{
		Type:  mgr.ServiceRestart,
		Delay: 1 * time.Minute,
	},
	{
		Type:  mgr.ServiceRestart,
		Delay: 1 * time.Minute,
	},
}

// PrepareInstaller prepares the installer
func PrepareInstaller(_ context.Context) error {
	return nil
}

// SetupInstaller installs and starts the installer
func SetupInstaller(_ context.Context) error {
	fmt.Println("Creating the installer service")
	// create installer service
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// remove the installer service if it already exists
	stable, err := m.OpenService("Datadog Installer")
	if err == nil {
		// service already exists
		err = stable.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete service: %w", err)
		}
		stable.Close()
		fmt.Println("Service already exists ... deleting")
	}

	// remove the experment service if it already exists
	exp, err := m.OpenService("Datadog Installer Experiment")
	if err == nil {
		// service already exists
		err = exp.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete service: %w", err)
		}
		exp.Close()
		fmt.Println("Service already exists ... deleting")
	}

	// create the installer service
	stable, err = m.CreateService("Datadog Installer", paths.StableInstallerPath, mgr.Config{
		DisplayName:      "Datadog Installer",
		Description:      "Datadog Installer",
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true,
		ServiceStartName: "LocalSystem",
	}, "run")
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer stable.Close()

	err = stable.SetRecoveryActions(stableRecoveryOptions, 0)
	if err != nil {
		return fmt.Errorf("failed to set recovery actions: %w", err)
	}

	err = stable.Start()
	if err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// create experiment service
	fmt.Println("Creating the installer experiment service")
	exp, err = m.CreateService("Datadog Installer Experiment", paths.ExperimentInstallerPath, mgr.Config{
		DisplayName:      "Datadog Installer Experiment",
		Description:      "Datadog Installer Experiment",
		StartType:        mgr.StartManual,
		DelayedAutoStart: true,
		ServiceStartName: "LocalSystem",
	}, "run")
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer exp.Close()

	return nil
}

func stopService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %w", err)
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("could not send stop control: %w", err)
	}

	timeout := time.Now().Add(30 * time.Second)
	for status.State != svc.Stopped {
		if time.Now().After(timeout) {
			return fmt.Errorf("timeout waiting for service to stop")
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %w", err)
		}
	}

	return nil
}

func startService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("could not access service: %w", err)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("could not start service: %w", err)
	}

	return nil
}

// RemoveInstaller removes the installer
func RemoveInstaller(_ context.Context) error {
	return msi.RemoveProduct("Datadog Installer")
}

// StartInstallerExperiment starts the installer experiment
func StartInstallerExperiment(_ context.Context) error {
	_, err := os.MkdirTemp(paths.RootTmpDir, "datadog-installer")
	if err != nil {
		return err
	}

	fmt.Println("Starting the installer experiment")

	// stop the stable installer
	if err := stopService("Datadog Installer"); err != nil {
		return fmt.Errorf("failed to stop stable installer: %w", err)
	}

	// start the experiment installer
	err = startService("Datadog Installer Experiment")
	if err != nil {
		return fmt.Errorf("failed to start experiment installer: %w", err)
	}

	// watch the service to make sure it continues to run for an hour
	// this is to ensure the experiment is running
	// if the service stops, the experiment will be stopped
	// and the stable installer will be started

	timeout := time.Now().Add(1 * time.Hour)
	status, err := watchServiceStatus("Datadog Installer Experiment", timeout, svc.Stopped)
	if err != nil {
		return fmt.Errorf("failed to watch service status: %w", err)
	}

	if !status {
		// the service never stopped and hit the timeout
		// this means we never got the promote signal
		// we should restore to the stable installer
		// stop the experiment installer
		fmt.Println("Experiment not stopped")
		err = stopService("Datadog Installer Experiment")
		if err != nil {
			return fmt.Errorf("failed to stop experiment installer: %w", err)
		}

		// start the stable installer
		err = startService("Datadog Installer")
		if err != nil {
			return fmt.Errorf("failed to start stable installer: %w", err)
		}
	}

	// make sure the stable installer starts
	timeout = time.Now().Add(5 * time.Minute)
	status, err = watchServiceStatus("Datadog Installer", timeout, svc.Running)
	if !status {
		// the stable installer never started which means we need to start it ourselves
		// this means the experiment probably crashed
		fmt.Println("Stable installer never started")
		err = startService("Datadog Installer")
		if err != nil {
			return fmt.Errorf("failed to start stable installer: %w", err)
		}
	}

	return nil

}

// watchServiceStatus watches the service to make sure it starts
func watchServiceStatus(name string, timeout time.Time, expStatus svc.State) (bool, error) {
	fmt.Println("Watching service status")
	fmt.Printf("Service: %s\n", name)
	fmt.Printf("Expected status: %d\n", expStatus)
	fmt.Printf("Timeout: %s\n", timeout)
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return false, fmt.Errorf("could not access service: %w", err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return false, fmt.Errorf("could not retrieve service status: %w", err)
	}

	for status.State != expStatus {
		if time.Now().After(timeout) {
			return false, nil
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return false, fmt.Errorf("could not retrieve service status: %w", err)
		}
	}

	return true, nil
}

// StopInstallerExperiment stops the installer experiment
func StopInstallerExperiment(_ context.Context) error {
	_, err := os.MkdirTemp(paths.RootTmpDir, "datadog-installer")
	if err != nil {
		return err
	}

	// stop the experiment installer
	if err := stopService("Datadog Installer Experiment"); err != nil {
		return fmt.Errorf("failed to stop experiment installer: %w", err)
	}

	// start the stable installer
	err = startService("Datadog Installer")
	if err != nil {
		return fmt.Errorf("failed to start stable installer: %w", err)
	}

	return nil

}

// PromoteInstallerExperiment promotes the installer experiment
func PromoteInstallerExperiment(_ context.Context) error {
	// start the stable installer
	if err := startService("Datadog Installer"); err != nil {
		return fmt.Errorf("failed to start stable installer: %w", err)
	}

	return nil
}

// StopAll stops all installers so the stable and experiment can be switched
func StopAll(_ context.Context) error {
	// stop the stable installer
	_ = stopService("Datadog Installer")

	// stop the experiment installer
	_ = stopService("Datadog Installer Experiment")

	return nil
}
