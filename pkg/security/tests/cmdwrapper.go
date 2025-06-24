// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

type wrapperType string

const (
	noWrapperType     wrapperType = "" //nolint:deadcode,unused
	stdWrapperType    wrapperType = "std"
	dockerWrapperType wrapperType = "docker"
	podmanWrapperType wrapperType = "podman"
	multiWrapperType  wrapperType = "multi"
)

// Because of rate limits, we allow the specification of multiple images for the same "kind".
// Since dockerhub limits per pulls by 6 hours, and aws limits by data over a month, we first try the dockerhub
// one and fallback on aws.
var dockerImageLibrary = map[string][]string{
	"ubuntu": {
		"ubuntu:20.04",
		"public.ecr.aws/ubuntu/ubuntu:20.04",
	},
	"centos": {
		"centos:7",
		"public.ecr.aws/docker/library/centos:7",
	},
	"alpine": {
		"alpine:3.18.2",
		"public.ecr.aws/docker/library/alpine:3.18.2", // before changing the version make sure that the new version behaves as previously (hardlink vs symlink)
	},
	"busybox": {
		"busybox:1.36.1",
		"docker.io/busybox:1.36.1", // before changing the version make sure that the new version behaves as previously (hardlink vs symlink)
	},
}

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
	mountSrc      string
	mountDest     string
	pid           int64
	containerName string
	containerID   string
	cgroupID      string
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
	d.containerName = fmt.Sprintf("docker-wrapper-%s", utils.RandString(6))
	cmd := exec.Command(d.executable, "run", "--cap-add=SYS_PTRACE", "--security-opt", "seccomp=unconfined", "--rm", "--cap-add", "NET_ADMIN", "-d", "--name", d.containerName, "-v", d.mountSrc+":"+d.mountDest, d.image, "sleep", "1200")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	d.containerID = strings.TrimSpace(string(out))

	cmd = exec.Command(d.executable, "inspect", "--format", "{{ .State.Pid }}", d.containerID)
	out, err = cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	d.pid, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)

	if d.cgroupID, err = getPIDCGroup(uint32(d.pid)); err != nil {
		return nil, err
	}

	return out, err
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

func (d *dockerCmdWrapper) selectImageFromLibrary(kind string) error {
	var err error
	for _, entry := range dockerImageLibrary[kind] {
		cmd := exec.Command(d.executable, "pull", entry)
		err = cmd.Run()
		if err == nil {
			d.image = entry
			break
		}
	}
	return err
}

func newDockerCmdWrapper(mountSrc, mountDest string, kind string, runtimeCommand string) (*dockerCmdWrapper, error) {
	if runtimeCommand == "" {
		runtimeCommand = "docker"
	}

	executable, err := exec.LookPath(runtimeCommand)
	if err != nil {
		return nil, err
	}

	// check docker is available
	cmd := exec.Command(executable, "version")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(strings.ToLower(string(output)), "\n") {
		splited := strings.SplitN(line, ":", 2)
		if splited[0] == "client" && len(splited) > 1 {
			if client := strings.TrimSpace(splited[1]); client != "" && !strings.Contains(client, runtimeCommand) {
				return nil, fmt.Errorf("client doesn't report as '%s' but as '%s'", runtimeCommand, client)
			}
		}
	}

	wrapper := &dockerCmdWrapper{
		executable: executable,
		mountSrc:   mountSrc,
		mountDest:  mountDest,
	}

	if err := wrapper.selectImageFromLibrary(kind); err != nil {
		return nil, err
	}

	return wrapper, nil
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
