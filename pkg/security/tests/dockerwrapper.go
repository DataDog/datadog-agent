// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build functionaltests

package tests

import (
	"os"
	"os/exec"
	"testing"
)

type dockerWrapper struct {
	executable string
}

func (d *dockerWrapper) Command(bin string, args []string, envs []string) *exec.Cmd {
	dockerArgs := []string{"exec"}
	for _, env := range envs {
		dockerArgs = append(dockerArgs, "-e"+env)
	}
	dockerArgs = append(dockerArgs, "docker-wrapper", bin)
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.Command(d.executable, dockerArgs...)
	cmd.Env = envs

	return cmd
}

func (d *dockerWrapper) Start() error {
	cmd := exec.Command(d.executable, "run", "-d", "--name", "docker-wrapper", "ubuntu:focal", "sleep", "600")
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}

	return nil
}

func (d *dockerWrapper) Stop() error {
	cmd := exec.Command(d.executable, "kill", "docker-wrapper")
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}

	cmd = exec.Command(d.executable, "rm", "docker-wrapper")
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}

	return nil
}

func (d *dockerWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Run(name, func(t *testing.T) {
		fnc(t, d.Command)
	})
}

func newDockerWrapper() *dockerWrapper {
	executable := "docker"
	if resolved, err := os.Readlink(executable); err == nil {
		executable = resolved
	}

	return &dockerWrapper{
		executable: executable,
	}
}
