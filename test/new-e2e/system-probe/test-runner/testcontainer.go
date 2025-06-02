// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
)

const containerName = "kmt-test-container"

type testContainer struct {
	image  string
	bpfDir string
}

func newTestContainer(image, bpfDir string) *testContainer {
	return &testContainer{
		image:  image,
		bpfDir: bpfDir,
	}
}

func (ctc *testContainer) runDockerCmd(args []string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (ctc *testContainer) start() error {
	args := []string{
		"run",
		"--name", containerName,
		"--privileged",
		"--detach",
	}

	var capabilities = []string{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "IPC_LOCK", "ALL"}
	for _, cap := range capabilities {
		args = append(args, "--cap-add", cap)
	}

	var mounts = []string{
		"/dev:/dev",
		"/proc:/host/proc",
		"/etc:/host/etc",
		"/sys:/host/sys",
		"/etc/os-release:/host/etc/os-release",
		"/usr/lib/os-release:/host/usr/lib/os-release",
		"/etc/passwd:/etc/passwd",
		"/etc/group:/etc/group",
		"/opt/datadog-agent/embedded/:/opt/datadog-agent/embedded/",
		"/opt/kmt-ramfs:/opt/kmt-ramfs",
		fmt.Sprintf("%s:/opt/bpf", ctc.bpfDir),
	}
	for _, mount := range mounts {
		args = append(args, "-v", mount)
	}

	var envs = []string{
		"HOST_PROC=/host/proc",
		"HOST_ETC=/host/etc",
		"HOST_SYS=/host/sys",
	}
	for _, env := range envs {
		args = append(args, "-e", env)
	}

	// create container
	args = append(args, ctc.image) // image tag
	args = append(args, "sleep", "infinity")
	if err := ctc.runDockerCmd(args); err != nil {
		return fmt.Errorf("run docker: %s", err)
	}

	// mount debugfs
	args = []string{"exec", containerName, "mount", "-t", "debugfs", "none", "/sys/kernel/debug"}
	if err := ctc.runDockerCmd(args); err != nil {
		return fmt.Errorf("run docker: %w", err)
	}

	return nil
}

func (ctc *testContainer) buildDockerExecArgs(testSuite string, envVars []string) []string {
	args := []string{"docker", "exec"}
	for _, envVar := range envVars {
		args = append(args, "-e", envVar)
	}
	args = append(args, containerName, testSuite)
	return args
}
