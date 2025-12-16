// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

type wrapperType string

const (
	noWrapperType      wrapperType = "" //nolint:deadcode,unused
	stdWrapperType     wrapperType = "std"
	dockerWrapperType  wrapperType = "docker"
	podmanWrapperType  wrapperType = "podman"
	systemdWrapperType wrapperType = "systemd"
	multiWrapperType   wrapperType = "multi"
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
		"public.ecr.aws/docker/library/busybox:1.36.1", // before changing the version make sure that the new version behaves as previously (hardlink vs symlink)
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
	return d.CommandContext(context.TODO(), bin, args, envs)
}

func (d *dockerCmdWrapper) CommandContext(ctx context.Context, bin string, args []string, envs []string) *exec.Cmd {
	dockerArgs := []string{"exec"}
	for _, env := range envs {
		dockerArgs = append(dockerArgs, "-e"+env)
	}
	dockerArgs = append(dockerArgs, d.containerName, bin)
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, d.executable, dockerArgs...)
	cmd.Env = envs

	return cmd
}

func (d *dockerCmdWrapper) start() ([]byte, error) {
	d.containerName = "docker-wrapper-" + utils.RandString(6)
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
	var output []byte
	for _, entry := range dockerImageLibrary[kind] {
		cmd := exec.Command(d.executable, "pull", entry)
		output, err = cmd.CombinedOutput()
		if err == nil {
			d.image = entry
			break
		}
	}
	if err != nil {
		return fmt.Errorf("%w, cmd output:\n%s", err, string(output))
	}
	return nil
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w, cmd output:\n%s", err, string(output))
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

type systemdCmdWrapper struct {
	serviceName string
	cgroupID    string
	reloadCmd   string
}

func (s *systemdCmdWrapper) Command(bin string, args []string, envs []string) *exec.Cmd {
	return s.CommandContext(context.TODO(), bin, args, envs)
}

func (s *systemdCmdWrapper) CommandContext(ctx context.Context, bin string, args []string, envs []string) *exec.Cmd {
	// Execute command within the systemd service context using systemd-run
	systemdArgs := []string{"--slice=" + s.serviceName, bin}
	systemdArgs = append(systemdArgs, args...)

	cmd := exec.CommandContext(ctx, "systemd-run", systemdArgs...)
	cmd.Env = envs
	return cmd
}

func (s *systemdCmdWrapper) reload() error {
	cmd := exec.Command("systemctl", "reload", s.serviceName)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload service: %v", err)
	}
	return nil
}

func (s *systemdCmdWrapper) start() ([]byte, error) {

	// Create a systemd service unit file
	serviceUnit := fmt.Sprintf(`[Unit]
Description=CWS Test Service %s
After=network.target

[Service]
Type=simple
ExecStart=/bin/sleep 3600
Restart=on-failure
RestartSec=1s
ExecReload=%s

[Install]
WantedBy=multi-user.target
`, s.serviceName, s.reloadCmd)

	serviceFile := "/etc/systemd/system/" + s.serviceName + ".service"
	if err := os.WriteFile(serviceFile, []byte(serviceUnit), 0644); err != nil {
		return nil, fmt.Errorf("failed to create service file: %v", err)
	}

	// Reload systemd and start the service
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return nil, fmt.Errorf("failed to reload systemd: %v", err)
	}

	if err := exec.Command("systemctl", "enable", s.serviceName).Run(); err != nil {
		return nil, fmt.Errorf("failed to enable service: %v", err)
	}

	cmd := exec.Command("systemctl", "start", s.serviceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("failed to start service: %v", err)
	}

	// Wait for the service to be running
	time.Sleep(2 * time.Second)

	// Get the cgroup ID for the service
	s.cgroupID = "/system.slice/" + s.serviceName + ".service"

	return out, nil
}

func (s *systemdCmdWrapper) stop() ([]byte, error) {
	if s == nil {
		return nil, nil
	}
	// Stop the systemd service
	cmd := exec.Command("systemctl", "stop", s.serviceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("failed to stop service: %v", err)
	}

	err = exec.Command("systemctl", "disable", s.serviceName).Run()
	if err != nil {
		return nil, fmt.Errorf("failed to disable service: %v", err)
	}

	err = os.Remove("/etc/systemd/system/" + s.serviceName + ".service")
	if err != nil {
		return nil, fmt.Errorf("failed to remove service file: %v", err)
	}

	err = exec.Command("systemctl", "daemon-reload").Run()
	if err != nil {
		return nil, fmt.Errorf("failed to reload systemd: %v", err)
	}

	return out, nil
}

func (s *systemdCmdWrapper) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	t.Run(name, func(t *testing.T) {
		fnc(t, s.Type(), s.Command)
	})
}

func (s *systemdCmdWrapper) Type() wrapperType {
	return systemdWrapperType
}

func newSystemdCmdWrapper(serviceName string, reloadCmd string) (*systemdCmdWrapper, error) {
	// Check if systemctl is available
	_, err := exec.LookPath("systemctl")
	if err != nil {
		return nil, err
	}

	// Check if systemd is running
	cmd := exec.Command("systemctl", "is-system-running")
	output, err := cmd.Output()
	if err != nil {
		state := strings.TrimSpace(string(output))
		if state == "running" || state == "degraded" {
			return &systemdCmdWrapper{serviceName: serviceName, reloadCmd: reloadCmd}, nil
		}
		return nil, fmt.Errorf("systemd is not running: %s", state)
	}

	return &systemdCmdWrapper{serviceName: serviceName, reloadCmd: reloadCmd}, nil
}
