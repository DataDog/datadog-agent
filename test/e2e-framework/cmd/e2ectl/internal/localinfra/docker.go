// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localinfra

import (
	"fmt"
	"os"
	"os/exec"
)

// Container and network names are deterministic functions of the environment
// name, so teardown never needs bookkeeping: if a start half-failed, `stop`
// can still name and remove everything that may exist.
func NetworkName(env string) string         { return env + "-net" }
func FakeintakeContainer(env string) string { return env + "-fakeintake" }
func AgentContainer(env string) string      { return env + "-agent" }

// CreateNetwork creates a Docker network (idempotent: an existing network is
// reused, because a retry after a failed start must recover, not collide).
func CreateNetwork(name string) error {
	cmd := exec.Command("docker", networkCreateArgs(name)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func networkCreateArgs(name string) []string { return []string{"network", "create", name} }

// RemoveNetwork removes a Docker network, ignoring "not found" (the same
// best-effort contract as StopFakeintake: a missing network is not an error).
func RemoveNetwork(name string) error {
	cmd := exec.Command("docker", networkRemoveArgs(name)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func networkRemoveArgs(name string) []string { return []string{"network", "rm", name} }

// RemoveContainer force-removes a container, ignoring "not found" (the
// installer may never have run; the container is the process handle).
func RemoveContainer(name string) error {
	cmd := exec.Command("docker", containerRemoveArgs(name)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func containerRemoveArgs(name string) []string { return []string{"rm", "-f", name} }

// RunFakeintakeOnNetwork starts the fakeintake container attached to a Docker
// network — the container-network twin of RunFakeintake. Agent containers on
// the same network reach it as <name>:80 via Docker DNS, while the published
// port keeps the operator's 127.0.0.1 inspection path.
func RunFakeintakeOnNetwork(container, network string) (int, error) {
	port, err := freePort()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command("docker", fakeintakeRunArgs(container, network, port)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("starting fakeintake: %w", err)
	}
	return port, nil
}

func fakeintakeRunArgs(container, network string, port int) []string {
	return []string{
		"run", "-d", "--name", container,
		"--network", network,
		"-p", fmt.Sprintf("%d:80", port),
		FakeintakeImage,
		"--rc-key-data=" + DefaultRCSigningKeySeed,
	}
}
