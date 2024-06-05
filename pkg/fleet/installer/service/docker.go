// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type dockerDaemonConfig map[string]interface{}

var (
	dockerDaemonPath = "/etc/docker/daemon.json"
)

func (a *apmInjectorInstaller) setupDocker(ctx context.Context) (rollback func() error, err error) {
	err = os.MkdirAll("/etc/docker", 0755)
	if err != nil {
		return nil, err
	}

	rollbackDockerConfig, err := a.dockerConfigInstrument.mutate(ctx)
	if err != nil {
		return nil, err
	}

	rollback = func() error {
		if rollbackDockerConfig == nil {
			return nil
		}
		if err := rollbackDockerConfig(); err != nil {
			return err
		}
		return reloadDockerConfig(ctx)
	}

	return rollback, reloadDockerConfig(ctx)
}

func (a *apmInjectorInstaller) uninstallDocker(ctx context.Context) error {
	if !isDockerInstalled(ctx) {
		return nil
	}
	if _, err := a.dockerConfigUninstrument.mutate(ctx); err != nil {
		return err
	}
	return reloadDockerConfig(ctx)
}

// setDockerConfigContent sets the content of the docker daemon configuration
func (a *apmInjectorInstaller) setDockerConfigContent(ctx context.Context, previousContent []byte) ([]byte, error) {
	span, _ := tracer.StartSpanFromContext(ctx, "set_docker_config_content")
	defer span.Finish()

	dockerConfig := dockerDaemonConfig{}

	if len(previousContent) > 0 {
		err := json.Unmarshal(previousContent, &dockerConfig)
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
func (a *apmInjectorInstaller) deleteDockerConfigContent(_ context.Context, previousContent []byte) ([]byte, error) {
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
func (a *apmInjectorInstaller) verifyDockerRuntime(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "verify_docker_runtime")
	defer span.Finish(tracer.WithError(err))
	for i := 0; i < 3; i++ {
		if i > 0 {
			time.Sleep(time.Second)
		}
		cmd := exec.Command("docker", "system", "info", "--format", "{{ .DefaultRuntime }}")
		var outb bytes.Buffer
		cmd.Stdout = &outb
		err = cmd.Run()
		if err != nil {
			if i < 2 {
				log.Debug("failed to verify docker runtime, retrying: ", err)
			} else {
				log.Warn("failed to verify docker runtime: ", err)
			}
		}
		if strings.TrimSpace(outb.String()) == "dd-shim" {
			return nil
		}
	}
	err = fmt.Errorf("docker default runtime has not been set to injector docker runtime")
	return err
}

func reloadDockerConfig(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "reload_docker")
	defer span.Finish(tracer.WithError(err))
	return exec.CommandContext(ctx, "systemctl", "reload", "docker").Run()
}

// isDockerInstalled checks if docker is installed on the system
func isDockerInstalled(ctx context.Context) bool {
	span, _ := tracer.StartSpanFromContext(ctx, "is_docker_installed")
	defer span.Finish()
	cmd := exec.CommandContext(ctx, "which", "docker")
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	span.SetTag("is_installed", err == nil)
	if err != nil {
		log.Warn("installer: failed to check if docker is installed, assuming it isn't: ", err)
		return false
	}
	if len(outb.String()) == 0 {
		log.Warn("installer: docker is not installed on the systemd, skipping docker configuration")
		return false
	}
	return true
}
