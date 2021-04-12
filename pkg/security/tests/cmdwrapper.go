// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build functionaltests

package tests

import (
	"fmt"
	"os/exec"
	"testing"
)

type wrapperType string

const (
	stdWrapperType    wrapperType = "std"
	dockerWrapperType             = "docker"
	multiWrapperType              = "multi"
	skipWrapperType               = "skip"
)

type cmdWrapper interface {
	Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd))
	Type() wrapperType
}

type stdCmdWrapper struct {
}

func (s *stdCmdWrapper) Command(bin string, args []string, envs []string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	cmd.Env = envs

	return cmd
}

func (s *stdCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Run(name, func(t *testing.T) {
		fnc(t, s.Type(), s.Command)
	})
}

func (s *stdCmdWrapper) Type() wrapperType {
	return stdWrapperType
}

func newStdCmdWrapper() *stdCmdWrapper {
	return &stdCmdWrapper{}
}

type dockerCmdWrapper struct {
	t          *testing.T
	executable string
	root       string
	isStarted  bool
}

func (d *dockerCmdWrapper) Command(bin string, args []string, envs []string) *exec.Cmd {
	if !d.isStarted {
		if out, err := d.start(); err != nil {
			d.t.Fatalf("%s: %s", string(out), err)
		}
		d.isStarted = true
	}

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

func (d *dockerCmdWrapper) start() ([]byte, error) {
	cmd := exec.Command(d.executable, "run", "-d", "--name", "docker-wrapper", "-v", d.root+":"+d.root, "ubuntu:focal", "sleep", "600")
	if out, err := cmd.CombinedOutput(); err != nil {
		return out, err
	}
	return nil, nil
}

func (d *dockerCmdWrapper) Stop() ([]byte, error) {
	cmd := exec.Command(d.executable, "kill", "docker-wrapper")
	if out, err := cmd.CombinedOutput(); err != nil {
		return out, err
	}

	cmd = exec.Command(d.executable, "rm", "docker-wrapper")
	if out, err := cmd.CombinedOutput(); err != nil {
		return out, err
	}

	return nil, nil
}

func (d *dockerCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Run(name, func(t *testing.T) {
		fnc(t, d.Type(), d.Command)
	})
}

func (d *dockerCmdWrapper) Type() wrapperType {
	return dockerWrapperType
}

func newDockerCmdWrapper(t *testing.T, root string) (*dockerCmdWrapper, error) {
	executable, err := exec.LookPath("docker")
	if err != nil {
		return nil, err
	}

	return &dockerCmdWrapper{
		t:          t,
		executable: executable,
		root:       root,
	}, nil
}

type skipCmdWrapper struct {
	reason string
}

func (s *skipCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Skip(s.reason)
}

func (s *skipCmdWrapper) Type() wrapperType {
	return skipWrapperType
}

func newSkipCmdWrapper(reason string) *skipCmdWrapper {
	return &skipCmdWrapper{
		reason: reason,
	}
}

type multiCmdWrapper struct {
	wrappers []cmdWrapper
}

func (m *multiCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	for _, wrapper := range m.wrappers {
		kind := wrapper.Type()
		wrapper.Run(t, fmt.Sprintf("%s-%s", name, kind), fnc)
	}
}

func (m *multiCmdWrapper) Type() wrapperType {
	return multiWrapperType
}

func newMultiCmdWrapper(wrappers ...cmdWrapper) *multiCmdWrapper {
	return &multiCmdWrapper{
		wrappers: wrappers,
	}
}
