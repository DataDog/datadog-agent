// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localinfra

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// BlackholeContainer is the deterministic managed-sink container name, so a
// half-created environment is always recoverable with plain `stop`.
func BlackholeContainer(env string) string { return env + "-blackhole" }

// blackholeListenPort is the sink's in-network port. It is never published to
// the host: the sink is reachable only from containers on the env network.
const blackholeListenPort = 8080

// StageBlackholeBinary copies the running e2ectl executable into the
// environment directory so the sink's read-only mount survives e2ectl being
// rebuilt or moved between the sync that starts the sink and the environment's
// teardown. The binary path is resolved at runtime, never hardcoded.
func StageBlackholeBinary(envDir string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving the running e2ectl binary: %w", err)
	}
	src, err := os.Open(self)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dir := filepath.Join(envDir, "sink")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Atomic replacement keeps a previously mounted copy intact while a fresh
	// sync stages the current binary for the next container start.
	dst, err := os.CreateTemp(dir, ".e2ectl-*")
	if err != nil {
		return "", err
	}
	dstPath := dst.Name()
	defer func() {
		dst.Close()
		if err != nil {
			_ = os.Remove(dstPath)
		}
	}()
	// 0755, not 0700: the pinned runtime image runs unprivileged as dd-agent,
	// which must read and execute the staged binary from its read-only mount.
	if err = dst.Chmod(0o755); err != nil {
		return "", err
	}
	if _, err = io.Copy(dst, src); err != nil {
		return "", err
	}
	if err = dst.Close(); err != nil {
		return "", err
	}
	owned := filepath.Join(dir, "e2ectl")
	if err = os.Rename(dstPath, owned); err != nil {
		return "", err
	}
	return owned, nil
}

// StopBlackhole removes the managed sink container (no error when absent).
func StopBlackhole(container string) error {
	_ = exec.Command("docker", "rm", "-f", container).Run()
	return nil
}

// RunBlackholeOnNetwork starts the environment-managed blackhole sink: the
// staged e2ectl binary mounted read-only into the pinned agent runtime image,
// serving `receiver serve` on every container interface. The container is
// read-only, unprivileged and publishes no host port — the sink exists only
// inside the environment's Docker network, for as long as an agent is routed
// to it.
func RunBlackholeOnNetwork(container, network, image, binary string) error {
	cmd := exec.Command("docker", blackholeRunArgs(container, network, image, binary)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("starting managed blackhole sink: %w", err)
	}
	return nil
}

func blackholeRunArgs(container, network, image, binary string) []string {
	return []string{
		"run", "-d", "--name", container,
		"--network", network,
		"--read-only", "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--no-healthcheck",
		"-v", binary + ":/tool:ro",
		"--entrypoint", "/tool",
		image,
		"receiver", "serve", "--type", "blackhole",
		"--listen", fmt.Sprintf("0.0.0.0:%d", blackholeListenPort),
	}
}

// BlackholeAgentURL is the in-network URL routed Agents use for the managed
// sink container.
func BlackholeAgentURL(env string) string {
	return fmt.Sprintf("http://%s:%d", BlackholeContainer(env), blackholeListenPort)
}
