// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	injectorConfigPrefix = []byte("\n# BEGIN LD PRELOAD CONFIG")
	injectorConfigSuffix = []byte("# END LD PRELOAD CONFIG\n")
)

const (
	injectorConfigTemplate = `
apm_config:
  receiver_socket: %s
use_dogstatsd: true
dogstatsd_socket: %s
`
	datadogConfigPath = "/etc/datadog-agent/datadog.yaml"
	ldSoPreloadPath   = "/etc/ld.so.preload"
)

// SetupAPMInjector sets up the injector at bootstrap
func SetupAPMInjector() error {
	// Enforce dd-installer is in the dd-agent group
	if err := setInstallerAgentGroup(); err != nil {
		return err
	}

	installer := &apmInjectorInstaller{
		installPath: "/opt/datadog-packages/datadog-apm-inject/stable",
	}
	return installer.Setup()
}

// RemoveAPMInjector removes the APM injector
func RemoveAPMInjector() error {
	installer := &apmInjectorInstaller{
		installPath: "/opt/datadog-packages/datadog-apm-inject/stable",
	}
	return installer.Remove()
}

type apmInjectorInstaller struct {
	installPath string
}

// Setup sets up the APM injector
func (a *apmInjectorInstaller) Setup() error {
	var err error
	defer func() {
		if err != nil {
			removeErr := a.Remove()
			if removeErr != nil {
				log.Warnf("Failed to remove APM injector: %v", removeErr)
			}
		}
	}()
	if err := a.setAgentConfig(); err != nil {
		return err
	}
	if err := a.setRunPermissions(); err != nil {
		return err
	}
	if err := a.setLDPreloadConfig(); err != nil {
		return err
	}
	if err := a.setDockerConfig(); err != nil {
		return err
	}
	return nil
}

func (a *apmInjectorInstaller) Remove() error {
	if err := a.deleteAgentConfig(); err != nil {
		return err
	}
	if err := a.deleteLDPreloadConfig(); err != nil {
		return err
	}
	if err := a.deleteDockerConfig(); err != nil {
		return err
	}
	return nil
}

func (a *apmInjectorInstaller) setRunPermissions() error {
	return os.Chmod(path.Join(a.installPath, "inject", "run"), 0777)
}

// setLDPreloadConfig adds preload options on /etc/ld.so.preload, overriding existing ones
func (a *apmInjectorInstaller) setLDPreloadConfig() error {
	launcherPreloadPath := path.Join(a.installPath, "inject", "launcher.preload.so")

	var ldSoPreload []byte
	stat, err := os.Stat(ldSoPreloadPath)
	if err == nil {
		ldSoPreload, err = os.ReadFile(ldSoPreloadPath)
		if err != nil {
			return err
		}
		if strings.Contains(string(ldSoPreload), launcherPreloadPath) {
			// If the line of interest is already in /etc/ld.so.preload, return fast
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Append the launcher preload path to the file
	if len(ldSoPreload) > 0 && ldSoPreload[len(ldSoPreload)-1] != '\n' {
		ldSoPreload = append(ldSoPreload, '\n')
	}
	ldSoPreload = append(ldSoPreload, []byte(launcherPreloadPath+"\n")...)

	perms := os.FileMode(0644)
	if stat != nil {
		perms = stat.Mode()
	}
	err = os.WriteFile("/tmp/ld.so.preload.tmp", ldSoPreload, perms)
	if err != nil {
		return err
	}

	return executeCommand(string(replaceLDPreloadCommand))
}

// deleteLDPreloadConfig removes the preload options from /etc/ld.so.preload
func (a *apmInjectorInstaller) deleteLDPreloadConfig() error {
	launcherPreloadPath := path.Join(a.installPath, "inject", "launcher.preload.so")

	var ldSoPreload []byte
	stat, err := os.Stat(ldSoPreloadPath)
	if err == nil {
		ldSoPreload, err = os.ReadFile(ldSoPreloadPath)
		if err != nil {
			return err
		}
		if !strings.Contains(string(ldSoPreload), launcherPreloadPath) {
			// If the line of interest isn't in /etc/ld.so.preload, return fast
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		return nil
	}

	// Remove the launcher preload path from the file
	ldSoPreload = bytes.Replace(ldSoPreload, []byte(launcherPreloadPath+"\n"), []byte{}, -1)

	perms := os.FileMode(0644)
	if stat != nil {
		perms = stat.Mode()
	}
	err = os.WriteFile("/tmp/ld.so.preload.tmp", ldSoPreload, perms)
	if err != nil {
		return err
	}

	return executeCommand(string(replaceLDPreloadCommand))
}

// setAgentConfig adds the agent configuration for the APM injector if it is not there already
// We assume that the agent file has been created by the installer's postinst script
//
// Note: This is not safe, as it assumes there were no changes to the agent configuration made without
// restart by the user. This means that the agent can crash on restart. This is a limitation of the current
// installer system and this will be replaced by a proper experiment when available. This is a temporary
// solution to allow the APM injector to be installed, and if the agent crashes, we try to detect it and
// restore the previous configuration
func (a *apmInjectorInstaller) setAgentConfig() (err error) {
	runPath := path.Join(a.installPath, "inject", "run")
	apmSocketPath := path.Join(runPath, "apm.socket")
	dsdSocketPath := path.Join(runPath, "dsd.socket")

	err = backupAgentConfig()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			restoreErr := restoreAgentConfig()
			if restoreErr != nil {
				log.Warnf("Failed to restore agent config: %v", restoreErr)
			}
		}
	}()

	content, err := os.ReadFile(datadogConfigPath)
	if err != nil {
		return err
	}

	if !bytes.Contains(content, injectorConfigPrefix) {
		content = append(content, injectorConfigPrefix...)
		content = append(content, []byte(
			fmt.Sprintf(injectorConfigTemplate, apmSocketPath, dsdSocketPath),
		)...)
		content = append(content, injectorConfigSuffix...)
		err = os.WriteFile(datadogConfigPath, content, 0644)
		if err != nil {
			return err
		}
	}
	err = restartInjectorAgents()
	return
}

// deleteAgentConfig removes the agent configuration for the APM injector
func (a *apmInjectorInstaller) deleteAgentConfig() (err error) {
	err = backupAgentConfig()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			restoreErr := restoreAgentConfig()
			if restoreErr != nil {
				log.Warnf("Failed to restore agent config: %v", restoreErr)
			}
		}
	}()

	content, err := os.ReadFile(datadogConfigPath)
	if err != nil {
		return err
	}

	start := bytes.Index(content, injectorConfigPrefix)
	end := bytes.Index(content, injectorConfigSuffix) + len(injectorConfigSuffix)
	if start == -1 || end == -1 || start >= end {
		// Config not found
		return nil
	}

	content = append(content[:start], content[end:]...)
	err = os.WriteFile(datadogConfigPath, content, 0644)
	if err != nil {
		return err
	}

	return restartInjectorAgents()
}

// backupAgentConfig backs up the agent configuration
func backupAgentConfig() error {
	return executeCommandStruct(privilegeCommand{
		Command: string(backupCommand),
		Path:    datadogConfigPath,
	})
}

// restoreAgentConfig restores the agent configuration & restarts the agent
func restoreAgentConfig() error {
	err := executeCommandStruct(privilegeCommand{
		Command: string(restoreCommand),
		Path:    datadogConfigPath,
	})
	if err != nil {
		return err
	}
	return restartInjectorAgents()
}

// restartInjectorAgents restarts the agent units, both stable and experimental
func restartInjectorAgents() error {
	// Check that the agent is installed first
	if !isDatadogAgentInstalled() {
		log.Info("updater: datadog-agent is not installed, skipping restart")
		return nil
	}

	// Restart stable units
	if err := restartUnit("datadog-agent.service"); err != nil {
		return err
	}
	if err := restartUnit("datadog-agent-trace.service"); err != nil {
		return err
	}

	// Restart experimental units
	if err := restartUnit("datadog-agent-exp.service"); err != nil {
		return err
	}
	if err := restartUnit("datadog-agent-trace-exp.service"); err != nil {
		return err
	}
	return nil
}

// isDatadogAgentInstalled checks if the datadog agent is installed on the system
func isDatadogAgentInstalled() bool {
	cmd := exec.Command("which", "datadog-agent")
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	if err != nil {
		log.Warn("updater: failed to check if datadog-agent is installed, assuming it isn't: ", err)
		return false
	}
	return len(outb.String()) != 0
}
