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
)

// SetupAPMInjector sets up the injector at bootstrap
func SetupAPMInjector() error {
	var err error
	defer func() {
		if err != nil {
			removeErr := RemoveAPMInjector()
			if removeErr != nil {
				log.Warnf("Failed to remove APM injector: %v", removeErr)
			}
		}
	}()

	injectorPath := "/opt/datadog-packages/datadog-apm-inject/stable"

	if err = setInstallerAgentGroup(); err != nil {
		return err
	}

	err = addAgentConfig(injectorPath)
	if err != nil {
		return err
	}

	err = os.Chmod(path.Join(injectorPath, "inject", "run"), 0777)
	if err != nil {
		return err
	}

	err = setupLDPreload(injectorPath)
	if err != nil {
		return err
	}

	err = setupDockerDaemon(injectorPath)
	if err != nil {
		return err
	}

	return nil
}

// RemoveAPMInjector removes the APM injector
func RemoveAPMInjector() error {
	injectorPath := "/opt/datadog-packages/datadog-apm-inject/stable"

	err := removeAgentConfig()
	if err != nil {
		return err
	}

	err = removeLDPreload()
	if err != nil {
		return err
	}

	err = removeDockerDaemon(injectorPath)
	if err != nil {
		return err
	}
	return nil
}

// setupLDPreload adds preload options on /etc/ld.so.preload, overriding existing ones
func setupLDPreload(basePath string) error {
	launcherPreloadPath := path.Join(basePath, "inject", "launcher.preload.so")
	ldSoPreloadPath := "/etc/ld.so.preload"
	var ldSoPreload []byte
	if _, err := os.Stat(ldSoPreloadPath); err == nil {
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
	return writeLDPreload()
}

// removeLDPreload removes the preload options from /etc/ld.so.preload
func removeLDPreload() error {
	return executeCommand(string(removeLdPreloadCommand))
}

// writeLDPreload writes the content to /etc/ld.so.preload
func writeLDPreload() error {
	return executeCommand(string(setupLdPreloadCommand))
}

// addAgentConfig adds the agent configuration for the APM injector if it is not there already
// We assume that the agent file has been created by the installer's postinst script
//
// Note: This is not safe, as it assumes there were no changes to the agent configuration made without
// restart by the user. This means that the agent can crash on restart. This is a limitation of the current
// installer system and this will be replaced by a proper experiment when available. This is a temporary
// solution to allow the APM injector to be installed, and if the agent crashes, we try to detect it and
// restore the previous configuration
func addAgentConfig(basePath string) (err error) {
	runPath := path.Join(basePath, "inject", "run")
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

// removeAgentConfig removes the agent configuration for the APM injector
func removeAgentConfig() (err error) {
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
