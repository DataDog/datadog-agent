// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build functionaltests

package tests

import (
	"os/exec"
	"testing"
)

type cmdWrapper interface {
	Run(t *testing.T, name string, fnc func(t *testing.T, cmd func(bin string, args []string, envs []string) *exec.Cmd))
}

type stdCmdWrapper struct {
}

func (s *stdCmdWrapper) Command(bin string, args []string, envs []string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	cmd.Env = envs

	return cmd
}

func (s *stdCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Run(name, func(t *testing.T) {
		fnc(t, s.Command)
	})
}

func newStdCmdWrapper() *stdCmdWrapper {
	return &stdCmdWrapper{}
}

type dockerCmdWrapper struct {
	executable string
}

func (d *dockerCmdWrapper) Command(bin string, args []string, envs []string) *exec.Cmd {
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

func (d *dockerCmdWrapper) Start(root string) error {
	cmd := exec.Command(d.executable, "run", "-d", "--name", "docker-wrapper", "ubuntu:focal", "sleep", "600")
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}

	cmd = d.Command("mkdir", []string{"-p", root}, nil)
	return cmd.Run()
}

func (d *dockerCmdWrapper) Stop() error {
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

func (d *dockerCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Run(name, func(t *testing.T) {
		fnc(t, d.Command)
	})
}

func newDockerCmdWrapper() (*dockerCmdWrapper, error) {
	executable, err := exec.LookPath("docker")
	if err != nil {
		return nil, err
	}

	return &dockerCmdWrapper{
		executable: executable,
	}, nil
}

type skipCmdWrapper struct {
	reason string
}

func (s *skipCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Skip(s.reason)
}

func newSkipCmdWrapper(reason string) *skipCmdWrapper {
	return &skipCmdWrapper{
		reason: reason,
	}
}
