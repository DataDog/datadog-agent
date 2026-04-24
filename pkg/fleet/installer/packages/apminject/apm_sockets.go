// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.yaml.in/yaml/v2"
)

const (
	apmInstallerSocket    = "/var/run/datadog/apm.socket"
	statsdInstallerSocket = "/var/run/datadog/dsd.socket"
	apmInjectOldPath      = "/opt/datadog/apm/inject"
	envFilePath           = "/opt/datadog-packages/run/environment"
)

// Overridden in tests
var (
	agentConfigPath = "/etc/datadog-agent/datadog.yaml"
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
// If the agent has already configured sockets, it will return them
// to avoid dropping spans from already configured services
func getSocketsPath() (string, string, error) {
	apmSocket := apmInstallerSocket
	statsdSocket := statsdInstallerSocket

	rawCfg, err := os.ReadFile(agentConfigPath)
	if err != nil && os.IsNotExist(err) {
		return apmSocket, statsdSocket, nil
	} else if err != nil {
		return "", "", fmt.Errorf("error reading agent configuration file: %w", err)
	}

	var cfg socketConfig
	if err = yaml.Unmarshal(rawCfg, &cfg); err != nil {
		log.Warn("Failed to unmarshal agent configuration, using default installer sockets")
		return apmSocket, statsdSocket, nil
	}
	if cfg.ApmSocketConfig.ReceiverSocket != "" {
		apmSocket = cfg.ApmSocketConfig.ReceiverSocket
	}
	if cfg.DogstatsdSocket != "" {
		statsdSocket = cfg.DogstatsdSocket
	}
	return apmSocket, statsdSocket, nil
}

// configureSocketsEnv configures the sockets for the agent & injector
func (a *InjectorInstaller) configureSocketsEnv(ctx context.Context) (retErr error) {
	envFile := newFileMutator(envFilePath, setSocketEnvs, nil, nil)
	a.cleanups = append(a.cleanups, envFile.cleanup)
	rollback, err := envFile.mutate(ctx)
	if err != nil {
		return err
	}
	a.rollbacks = append(a.rollbacks, rollback)
	// Make sure the file is word readable
	if err = os.Chmod(envFilePath, 0644); err != nil {
		return fmt.Errorf("error changing permissions of %s: %w", envFilePath, err)
	}

	// Symlinks for sysvinit
	if err = os.Symlink(envFilePath, "/etc/default/datadog-agent-trace"); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to symlink %s to /etc/default/datadog-agent-trace: %w", envFilePath, err)
	}
	if err = os.Symlink(envFilePath, "/etc/default/datadog-agent"); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to symlink %s to /etc/default/datadog-agent: %w", envFilePath, err)
	}
	systemdRunning, err := systemd.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check if systemd is running: %w", err)
	}
	if systemdRunning {
		if err = addSystemDEnvOverrides(ctx, "datadog-agent.service"); err != nil {
			return err
		}
		if err = addSystemDEnvOverrides(ctx, "datadog-agent-exp.service"); err != nil {
			return err
		}
		if err = addSystemDEnvOverrides(ctx, "datadog-agent-trace.service"); err != nil {
			return err
		}
		if err = addSystemDEnvOverrides(ctx, "datadog-agent-trace-exp.service"); err != nil {
			return err
		}
		if err = systemd.Reload(ctx); err != nil {
			return err
		}
	}

	return nil
}

// setSocketEnvs sets the socket environment variables
func setSocketEnvs(ctx context.Context, envFile []byte) (res []byte, err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "set_socket_envs")
	defer span.Finish(err)

	apmSocket, statsdSocket, err := getSocketsPath()
	if err != nil {
		return nil, fmt.Errorf("error getting sockets path: %w", err)
	}

	span.SetTag("socket_path.apm", apmSocket)
	span.SetTag("socket_path.dogstatsd", statsdSocket)

	envs := map[string]string{
		"DD_APM_RECEIVER_SOCKET": apmSocket,
		"DD_DOGSTATSD_SOCKET":    statsdSocket,
		"DD_USE_DOGSTATSD":       "true",
	}
	return addEnvsIfNotSet(envs, envFile)
}

// addEnvsIfNotSet adds environment variables to the environment file if they are not already set
func addEnvsIfNotSet(envs map[string]string, envFile []byte) ([]byte, error) {
	// Build a map of the existing env vars
	existingEnvs := map[string]bool{}
	for line := range strings.SplitSeq(string(envFile), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) < 2 {
			continue
		}
		existingEnvs[strings.TrimSpace(parts[0])] = true
	}

	var buffer bytes.Buffer
	buffer.Write(envFile)
	if len(envFile) > 0 && envFile[len(envFile)-1] != '\n' {
		buffer.WriteByte('\n')
	}
	for key, value := range envs {
		if !existingEnvs[key] {
			buffer.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		}
	}
	return buffer.Bytes(), nil
}

// addSystemDEnvOverrides adds /etc/datadog-agent/environment variables to the defined systemd units
// The unit should contain the .service suffix (e.g. datadog-agent-exp.service)
//
// Reloading systemd & restarting the unit has to be done separately by the caller
func addSystemDEnvOverrides(ctx context.Context, unit string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "add_systemd_env_overrides")
	defer func() { span.Finish(err) }()
	span.SetTag("unit", unit)

	// The - is important as it lets the unit start even if the file is missing.
	content := []byte(fmt.Sprintf("[Service]\nEnvironmentFile=-%s\n", envFilePath))

	// We don't need a file mutator here as we're fully hard coding the content.
	// We don't really need to remove the file either as it'll just be ignored once the
	// unit is removed.
	return systemd.WriteUnitOverride(ctx, unit, "datadog_environment", string(content))
}
