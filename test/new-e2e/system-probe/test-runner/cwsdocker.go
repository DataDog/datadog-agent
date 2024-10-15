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
	"path/filepath"
)

const cwsContainerName = "cws-tests"

type cwsTestContainer struct {
	pwd    string
	bpfDir string
}

func newCWSTestContainer(testsuite string, bpfDir string) *cwsTestContainer {
	return &cwsTestContainer{
		pwd:    filepath.Dir(testsuite),
		bpfDir: bpfDir,
	}
}

func (ctc *cwsTestContainer) runDockerCmd(args []string) error {
	cmd := exec.Command("docker", args...)
	cmd.Dir = ctc.pwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (ctc *cwsTestContainer) start() error {
	args := []string{
		"run",
		"--name", cwsContainerName,
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
		"DD_SYSTEM_PROBE_BPF_DIR=/opt/bpf",
	}
	for _, env := range envs {
		args = append(args, "-e", env)
	}

	// create container
	args = append(args, "ghcr.io/datadog/apps-cws-centos7:main") // image tag
	args = append(args, "sleep", "7200")
	if err := ctc.runDockerCmd(args); err != nil {
		return fmt.Errorf("run docker: %s", err)
	}

	// mount debugfs
	args = []string{"exec", cwsContainerName, "mount", "-t", "debugfs", "none", "/sys/kernel/debug"}
	if err := ctc.runDockerCmd(args); err != nil {
		return fmt.Errorf("run docker: %w", err)
	}

	return nil
}

func (ctc *cwsTestContainer) stopAndRemove() error {
	args := []string{"stop", cwsContainerName}
	if err := ctc.runDockerCmd(args); err != nil {
		return fmt.Errorf("run docker: %w", err)
	}

	args = []string{"rm", cwsContainerName}
	if err := ctc.runDockerCmd(args); err != nil {
		return fmt.Errorf("run docker: %w", err)
	}

	return nil
}

func (ctc *cwsTestContainer) buildDockerExecArgs(args ...string) []string {
	return append([]string{"docker", "exec", cwsContainerName}, args...)
}
