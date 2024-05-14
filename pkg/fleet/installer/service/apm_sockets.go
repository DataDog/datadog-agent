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
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)

const (
	apmInstallerSocket    = "/var/run/datadog/installer/apm.socket"
	statsdInstallerSocket = "/var/run/datadog/installer/dsd.socket"
	agentConfigPath       = "/etc/datadog-agent/datadog.yaml"
	apmInjectOldPath      = "/opt/datadog/apm/inject"
	apmOldSocket          = "/opt/datadog/apm/inject/run/apm.socket"
	statsdOldSocket       = "/opt/datadog/apm/inject/run/dsd.socket"
)

// socketConfig is a subset of the agent configuration
type socketConfig struct {
	ApmSocketConfig ApmSocketConfig `yaml:"apm_config"`
	UseDogstatsd    bool            `yaml:"use_dogstatsd"`
	DogstatsdSocket string          `yaml:"dogstatsd_socket"`
}

// ApmSocketConfig is a subset of the agent configuration
type ApmSocketConfig struct {
	ReceiverSocket string `yaml:"receiver_socket"`
}

// getSocketsPath returns the sockets path for the agent and the injector
// If the agent is installed with the old apm-inject, it will return the old sockets
// to avoid dropping spans from already configured services
// If not, it will return the default sockets
//
// Note that we don't handle the case where sockets are configured in the agent configuration
// but the old apm-inject is not setup. In this configuration, span drops are expected.
func getSocketsPath(agentConfigPath, apmOldPath string) (string, string) {
	apmSocket := apmInstallerSocket
	statsdSocket := statsdInstallerSocket

	// apmOldPath only exists if the agent is installed with the old apm-inject (deb/rpm)
	// so we can return early if it doesn't
	if _, err := os.Stat(apmOldPath); err == os.ErrNotExist {
		return apmSocket, statsdSocket
	}

	rawCfg, err := os.ReadFile(agentConfigPath)
	if err != nil {
		log.Warn("Failed to read agent configuration file, using default installer sockets")
		return apmSocket, statsdSocket
	}

	var cfg socketConfig
	if err := yaml.Unmarshal(rawCfg, &cfg); err != nil {
		log.Warn("Failed to unmarshal agent configuration, using default installer sockets")
		return apmSocket, statsdSocket
	}
	if cfg.ApmSocketConfig.ReceiverSocket == apmOldSocket {
		apmSocket = apmOldSocket
	}
	if cfg.DogstatsdSocket == statsdOldSocket {
		statsdSocket = statsdOldSocket
	}
	return apmSocket, statsdSocket
}

// configureSocketsEnv configures the sockets for the agent & injector
func configureSocketsEnv() error {
	envFile := newFileMutator("/etc/environment", setSocketEnvs, verifySocketEnvs, verifySocketEnvs)
	defer envFile.cleanup()
	rollback, err := envFile.mutate()
	if err != nil {
		rollbackErr := rollback()
		if rollbackErr != nil {
			log.Warnf("Failed to rollback environment file: %v", rollbackErr)
		}
		return fmt.Errorf("error configuring sockets: %w", err)
	}
	return nil
}

// setSocketEnvs sets the socket environment variables
func setSocketEnvs(envFile []byte) ([]byte, error) {
	apmSocket, statsdSocket := getSocketsPath(agentConfigPath, apmInjectOldPath)
	envs := map[string]string{
		"DD_APM_RECEIVER_SOCKET": apmSocket,
		"DD_DOGSTATSD_SOCKET":    statsdSocket,
		"DD_USE_DOGSTATSD":       "true",
	}
	return addEnvsIfNotSet(envs, envFile)
}

// addEnvsIfNotSet adds environment variables to the environment file if they are not already set
func addEnvsIfNotSet(envs map[string]string, envFile []byte) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.Write(envFile)

	if len(envFile) > 0 && envFile[len(envFile)-1] != '\n' {
		buffer.WriteByte('\n')
	}
	for key, value := range envs {
		// There's a slight gap where there exists an env var in the form of {prefix}{key} but not {key},
		// but as our env vars all start with DD_ prefixing them won't cause any issue.
		if !bytes.Contains(envFile, []byte(key+"=")) {
			buffer.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		}
	}
	return buffer.Bytes(), nil
}

// verifySocketEnvs verifies that socket environment variables can be sourced
func verifySocketEnvs(path string) error {
	// Verify DD_USE_DOGSTATSD
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("source %s && echo $DD_USE_DOGSTATSD", path))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error verifying socket environment variables: %w", err)
	}
	if string(output) != "true" {
		return fmt.Errorf("socket environment variables not set correctly")
	}

	// Verify DD_APM_RECEIVER_SOCKET
	cmd = exec.Command("/bin/sh", "-c", fmt.Sprintf("source %s && echo $DD_APM_RECEIVER_SOCKET", path))
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error verifying socket environment variables: %w", err)
	}
	if string(output) == "" {
		return fmt.Errorf("socket environment variables not set correctly")
	}

	// Verify DD_DOGSTATSD_SOCKET
	cmd = exec.Command("/bin/sh", "-c", fmt.Sprintf("source %s && echo $DD_DOGSTATSD_SOCKET", path))
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error verifying socket environment variables: %w", err)
	}
	if string(output) == "" {
		return fmt.Errorf("socket environment variables not set correctly")
	}
	return nil
}

// addSystemDEnvOverrides adds /etc/environment variables to the defined systemd units
// The unit should contain the .service suffix (e.g. datadog-agent-exp.service)
//
// Reloading systemd & restarting the unit has to be done separately by the caller
func addSystemDEnvOverrides(unit string) error {
	// Verify that the unit is a DD one
	if !strings.HasPrefix(unit, "datadog-") {
		return fmt.Errorf("unit %s is not a Datadog unit", unit)
	}
	if !strings.HasSuffix(unit, ".service") {
		return fmt.Errorf("unit %s is not a service unit", unit)
	}

	content := []byte("[Service]\nEnvironmentFile=/etc/environment\n")

	// We don't need a file mutator here as we're fully hard coding the content.
	// We don't really need to remove the file either as it'll just be ignored once the
	// unit is removed.
	path := fmt.Sprintf("/etc/systemd/system/%s.d/environment_override.conf", unit)
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("error creating systemd environment override directory: %w", err)
	}
	err = os.WriteFile(path, content, 0644)
	if err != nil {
		return fmt.Errorf("error writing systemd environment override: %w", err)
	}
	return nil
}
