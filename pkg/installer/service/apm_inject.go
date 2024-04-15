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
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	injectorConfigPrefix = []byte("# BEGIN LD PRELOAD CONFIG")
	injectorConfigSuffix = []byte("# END LD PRELOAD CONFIG")
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
	var ldSoPreload []byte
	stat, err := os.Stat(ldSoPreloadPath)
	if err == nil {
		ldSoPreload, err = os.ReadFile(ldSoPreloadPath)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	newLdSoPreload, err := a.setLDPreloadConfigContent(ldSoPreload)
	if err != nil {
		return err
	}
	if bytes.Equal(ldSoPreload, newLdSoPreload) {
		// No changes needed
		return nil
	}

	perms := os.FileMode(0644)
	if stat != nil {
		perms = stat.Mode()
	}
	err = os.WriteFile("/tmp/ld.so.preload.tmp", newLdSoPreload, perms)
	if err != nil {
		return err
	}

	return executeCommand(string(replaceLDPreloadCommand))
}

// setLDPreloadConfigContent sets the content of the LD preload configuration
func (a *apmInjectorInstaller) setLDPreloadConfigContent(ldSoPreload []byte) ([]byte, error) {
	launcherPreloadPath := path.Join(a.installPath, "inject", "launcher.preload.so")

	if strings.Contains(string(ldSoPreload), launcherPreloadPath) {
		// If the line of interest is already in /etc/ld.so.preload, return fast
		return ldSoPreload, nil
	}

	// Append the launcher preload path to the file
	if len(ldSoPreload) > 0 && ldSoPreload[len(ldSoPreload)-1] != '\n' {
		ldSoPreload = append(ldSoPreload, '\n')
	}
	ldSoPreload = append(ldSoPreload, []byte(launcherPreloadPath+"\n")...)
	return ldSoPreload, nil
}

// deleteLDPreloadConfig removes the preload options from /etc/ld.so.preload
func (a *apmInjectorInstaller) deleteLDPreloadConfig() error {
	var ldSoPreload []byte
	stat, err := os.Stat(ldSoPreloadPath)
	if err == nil {
		ldSoPreload, err = os.ReadFile(ldSoPreloadPath)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		return nil
	}

	newLdSoPreload, err := a.deleteLDPreloadConfigContent(ldSoPreload)
	if err != nil {
		return err
	}
	if bytes.Equal(ldSoPreload, newLdSoPreload) {
		// No changes needed
		return nil
	}

	perms := os.FileMode(0644)
	if stat != nil {
		perms = stat.Mode()
	}
	err = os.WriteFile("/tmp/ld.so.preload.tmp", newLdSoPreload, perms)
	if err != nil {
		return err
	}

	return executeCommand(string(replaceLDPreloadCommand))
}

// deleteLDPreloadConfigContent deletes the content of the LD preload configuration
func (a *apmInjectorInstaller) deleteLDPreloadConfigContent(ldSoPreload []byte) ([]byte, error) {
	launcherPreloadPath := path.Join(a.installPath, "inject", "launcher.preload.so")

	if !strings.Contains(string(ldSoPreload), launcherPreloadPath) {
		// If the line of interest isn't there, return fast
		return ldSoPreload, nil
	}

	// Possible configurations of the preload path, order matters
	replacementsToTest := [][]byte{
		[]byte(launcherPreloadPath + "\n"),
		[]byte("\n" + launcherPreloadPath),
		[]byte(launcherPreloadPath + " "),
		[]byte(" " + launcherPreloadPath),
	}
	for _, replacement := range replacementsToTest {
		ldSoPreloadNew := bytes.Replace(ldSoPreload, replacement, []byte{}, 1)
		if !bytes.Equal(ldSoPreloadNew, ldSoPreload) {
			return ldSoPreloadNew, nil
		}
	}
	if bytes.Equal(ldSoPreload, []byte(launcherPreloadPath)) {
		// If the line is the only one in the file without newlines, return an empty file
		return []byte{}, nil
	}

	return nil, fmt.Errorf("failed to remove %s from %s", launcherPreloadPath, ldSoPreloadPath)
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

	newContent := a.setAgentConfigContent(content)
	if bytes.Equal(content, newContent) {
		// No changes needed
		return nil
	}

	err = os.WriteFile(datadogConfigPath, newContent, 0644)
	if err != nil {
		return err
	}

	err = restartTraceAgent()
	return
}

func (a *apmInjectorInstaller) setAgentConfigContent(content []byte) []byte {
	runPath := path.Join(a.installPath, "inject", "run")
	apmSocketPath := path.Join(runPath, "apm.socket")
	dsdSocketPath := path.Join(runPath, "dsd.socket")

	if !bytes.Contains(content, injectorConfigPrefix) {
		content = append(content, []byte("\n")...)
		content = append(content, injectorConfigPrefix...)
		content = append(content, []byte(
			fmt.Sprintf(injectorConfigTemplate, apmSocketPath, dsdSocketPath),
		)...)
		content = append(content, injectorConfigSuffix...)
		content = append(content, []byte("\n")...)
	}
	return content
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

	newContent := a.deleteAgentConfigContent(content)
	if bytes.Equal(content, newContent) {
		// No changes needed
		return nil
	}

	err = os.WriteFile(datadogConfigPath, newContent, 0644)
	if err != nil {
		return err
	}

	return restartTraceAgent()
}

// deleteAgentConfigContent deletes the agent configuration for the APM injector
func (a *apmInjectorInstaller) deleteAgentConfigContent(content []byte) []byte {
	start := bytes.Index(content, injectorConfigPrefix)
	end := bytes.Index(content, injectorConfigSuffix) + len(injectorConfigSuffix)
	if start == -1 || end == -1 || start >= end {
		// Config not found
		return content
	}

	return append(content[:start], content[end:]...)
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
	return restartTraceAgent()
}

// restartTraceAgent restarts the trace agent, both stable and experimental
func restartTraceAgent() error {
	if err := restartUnit("datadog-agent-trace.service"); err != nil {
		return err
	}
	if err := restartUnit("datadog-agent-trace-exp.service"); err != nil {
		return err
	}
	return nil
}
