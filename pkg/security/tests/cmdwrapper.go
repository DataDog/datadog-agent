// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests
// +build functionaltests stresstests

package tests

import (
	"fmt"
	"os/exec"
	"testing"
)

type wrapperType string

const (
	stdWrapperType    wrapperType = "std"
	dockerWrapperType wrapperType = "docker"
	multiWrapperType  wrapperType = "multi"
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
	executable    string
	root          string
	containerName string
	image         string
}

func (d *dockerCmdWrapper) Command(bin string, args []string, envs []string) *exec.Cmd {
	dockerArgs := []string{"exec"}
	for _, env := range envs {
		dockerArgs = append(dockerArgs, "-e"+env)
	}
	dockerArgs = append(dockerArgs, d.containerName, bin)
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.Command(d.executable, dockerArgs...)
	cmd.Env = envs

	return cmd
}

func (d *dockerCmdWrapper) start() ([]byte, error) {
	d.containerName = fmt.Sprintf("docker-wrapper-%s", randStringRunes(6))
	cmd := exec.Command(d.executable, "run", "--rm", "-d", "--name", d.containerName, "-v", d.root+":"+d.root, d.image, "sleep", "600")
	return cmd.CombinedOutput()
}

func (d *dockerCmdWrapper) stop() ([]byte, error) {
	cmd := exec.Command(d.executable, "kill", d.containerName)
	return cmd.CombinedOutput()
}

func (d *dockerCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Run(name, func(t *testing.T) {
		// force stop in case of previous failure
		_, _ = d.stop()

		if out, err := d.start(); err != nil {
			t.Errorf("%s: %s", string(out), err)
			return
		}
		fnc(t, d.Type(), d.Command)
		if out, err := d.stop(); err != nil {
			t.Errorf("%s: %s", string(out), err)
			return
		}
	})
}

func (d *dockerCmdWrapper) Type() wrapperType {
	return dockerWrapperType
}

func newDockerCmdWrapper(root string, image ...string) (*dockerCmdWrapper, error) {
	executable, err := exec.LookPath("docker")
	if err != nil {
		return nil, err
	}

	// check docker is available
	cmd := exec.Command(executable, "version")
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	dw := &dockerCmdWrapper{
		executable: executable,
		root:       root,
		image:      "ubuntu:focal",
	}

	if len(image) > 0 && image[0] != "" {
		dw.image = image[0]
	}

	return dw, nil
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
