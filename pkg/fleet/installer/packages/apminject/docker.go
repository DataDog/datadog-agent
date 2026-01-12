// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dockerDaemonConfig map[string]interface{}

var (
	dockerDaemonPath = "/etc/docker/daemon.json"
)

// instrumentDocker instruments the docker runtime to use the APM injector.
func (a *InjectorInstaller) instrumentDocker(ctx context.Context) (func() error, error) {
	err := os.MkdirAll("/etc/docker", 0755)
	if err != nil {
		return nil, err
	}

	rollbackDockerConfig, err := a.dockerConfigInstrument.mutate(ctx)
	if err != nil {
		return nil, err
	}

	err = reloadDockerConfig(ctx)
	if err != nil {
		if rollbackErr := rollbackDockerConfig(); rollbackErr != nil {
			log.Warn("failed to rollback docker configuration: ", rollbackErr)
		}
		return nil, err
	}

	rollbackWithReload := func() error {
		if err := rollbackDockerConfig(); err != nil {
			return err
		}
		return reloadDockerConfig(ctx)
	}

	return rollbackWithReload, nil
}

// uninstrumentDocker removes the APM injector from the Docker runtime.
func (a *InjectorInstaller) uninstrumentDocker(ctx context.Context) error {
	if !isDockerInstalled(ctx) {
		return nil
	}
	if _, err := a.dockerConfigUninstrument.mutate(ctx); err != nil {
		return err
	}
	return reloadDockerConfig(ctx)
}

// setDockerConfigContent sets the content of the docker daemon configuration
func (a *InjectorInstaller) setDockerConfigContent(ctx context.Context, previousContent []byte) (res []byte, err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "set_docker_config_content")
	defer span.Finish(err)

	dockerConfig := dockerDaemonConfig{}

	if len(previousContent) > 0 {
		err = json.Unmarshal(previousContent, &dockerConfig)
		if err != nil {
			return nil, err
		}
	}
	span.SetTag("docker_config.previous.default_runtime", dockerConfig["default-runtime"])
	dockerConfig["default-runtime"] = "dd-shim"
	runtimes, ok := dockerConfig["runtimes"].(map[string]interface{})
	if !ok {
		runtimes = map[string]interface{}{}
	}
	span.SetTag("docker_config.previous.runtimes_count", len(runtimes))
	runtimes["dd-shim"] = map[string]interface{}{
		"path": path.Join(a.installPath, "inject", "auto_inject_runc"),
	}
	dockerConfig["runtimes"] = runtimes

	dockerConfigJSON, err := json.MarshalIndent(dockerConfig, "", "    ")
	if err != nil {
		return nil, err
	}

	return dockerConfigJSON, nil
}

// deleteDockerConfigContent restores the content of the docker daemon configuration
func (a *InjectorInstaller) deleteDockerConfigContent(_ context.Context, previousContent []byte) ([]byte, error) {
	dockerConfig := dockerDaemonConfig{}

	if len(previousContent) > 0 {
		err := json.Unmarshal(previousContent, &dockerConfig)
		if err != nil {
			return nil, err
		}
	}

	if defaultRuntime, ok := dockerConfig["default-runtime"].(string); ok && defaultRuntime == "dd-shim" || !ok {
		dockerConfig["default-runtime"] = "runc"
	}
	runtimes, ok := dockerConfig["runtimes"].(map[string]interface{})
	if !ok {
		runtimes = map[string]interface{}{}
	}
	delete(runtimes, "dd-shim")
	dockerConfig["runtimes"] = runtimes

	dockerConfigJSON, err := json.MarshalIndent(dockerConfig, "", "    ")
	if err != nil {
		return nil, err
	}

	return dockerConfigJSON, nil
}

// verifyDockerRuntime validates that docker runtime configuration contains
// a path to the injector runtime.
// As the reload is eventually consistent we have to retry a few times
//
// This method is valid since at least Docker 17.03 (last update 2018-08-30)
func (a *InjectorInstaller) verifyDockerRuntime(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "verify_docker_runtime")
	defer func() { span.Finish(err) }()

	if !isDockerActive(ctx) {
		log.Warn("docker is inactive, skipping docker runtime verification")
		return nil
	}

	currentRuntime := ""
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		cmd := telemetry.CommandContext(ctx, "docker", "system", "info", "--format", "{{ .DefaultRuntime }}")
		var outb bytes.Buffer
		cmd.Stdout = &outb
		err = cmd.Run()
		if err != nil {
			if i < maxRetries {
				log.Debug("failed to verify docker runtime, retrying: ", err)
			} else {
				log.Warn("failed to verify docker runtime: ", err)
			}
		}
		currentRuntime = strings.TrimSpace(outb.String())
		if currentRuntime == "dd-shim" {
			span.SetTag("retries", i)
			span.SetTag("docker_runtime", "dd-shim")
			return nil
		}
		// Reload Docker daemon again in case the signal was lost
		if reloadErr := reloadDockerConfig(ctx); reloadErr != nil {
			log.Warn("failed to reload docker daemon: ", reloadErr)
		}
	}
	span.SetTag("retries", maxRetries)
	span.SetTag("docker_runtime", currentRuntime)
	err = fmt.Errorf("docker default runtime has not been set to injector docker runtime (is \"%s\")", currentRuntime)
	return err
}

func reloadDockerConfig(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "reload_docker")
	defer func() { span.Finish(err) }()
	if !isDockerActive(ctx) {
		log.Warn("docker is inactive, skipping docker reload")
		return nil
	}

	pids := []int32{}
	processes, err := process.Processes()
	if err != nil {
		return fmt.Errorf("couldn't get running processes: %s", err.Error())
	}
	for _, process := range processes {
		name, err := process.NameWithContext(ctx)
		if err != nil {
			continue // Don't pollute with warning logs
		}
		if name == "dockerd" {
			pids = append(pids, process.Pid)
		}
	}
	span.SetTag("dockerd_count", len(pids))
	for _, pid := range pids {
		err = syscall.Kill(int(pid), syscall.SIGHUP)
		if err != nil {
			return fmt.Errorf("failed to reload docker daemon (pid %d): %s", pid, err.Error())
		}
	}
	return nil
}

// isDockerInstalled checks if docker is installed on the system
func isDockerInstalled(ctx context.Context) bool {
	span, _ := telemetry.StartSpanFromContext(ctx, "is_docker_installed")
	defer span.Finish(nil)

	// Docker is installed if the docker binary is in the PATH
	dockerPath, err := exec.LookPath("docker")
	if err != nil && errors.Is(err, exec.ErrNotFound) {
		return false
	} else if err != nil {
		log.Warn("installer: failed to check if docker is installed, assuming it isn't: ", err)
		return false
	}
	span.SetTag("docker_path", dockerPath)
	if strings.Contains(dockerPath, "/snap/") {
		log.Warn("installer: docker is installed via snap, skipping docker instrumentation")
		return false
	}
	return true
}

// isDockerActive checks if docker is started on the system
func isDockerActive(ctx context.Context) bool {
	processes, err := process.Processes()
	if err != nil {
		return false // Don't pollute with warning logs
	}
	for _, process := range processes {
		name, err := process.NameWithContext(ctx)
		if err != nil {
			continue // Don't pollute with warning logs
		}
		if name == "dockerd" {
			return true
		}
	}
	return false
}
