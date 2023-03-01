// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	DefaultTimeout = time.Minute
)

// RunDockerServer is a template for running a protocols server in a docker.
// - serverName is a friendly name of the server we are setting (AMQP, mongo, etc.).
// - dockerPath is the path for the docker-compose.
// - env is any environment variable required for running the server.
// - serverStartRegex is a regex to be matched on the server logs to ensure it started correctly.
// return true on success
func RunDockerServer(t *testing.T, serverName, dockerPath string, env []string, serverStartRegex *regexp.Regexp, timeout time.Duration) bool {
	t.Helper()

	cmd := exec.Command("docker-compose", "-f", dockerPath, "up")
	patternScanner := NewScanner(serverStartRegex, make(chan struct{}, 1))

	cmd.Stdout = patternScanner
	cmd.Stderr = patternScanner
	cmd.Env = append(cmd.Env, env...)
	go func() {
		require.NoErrorf(t, cmd.Run(), "could not start %s with docker-compose", serverName)
	}()

	t.Cleanup(func() {
		c := exec.Command("docker-compose", "-f", dockerPath, "down", "--remove-orphans")
		c.Env = append(c.Env, env...)
		_ = c.Run()
	})

	for {
		select {
		case <-patternScanner.DoneChan:
			t.Logf("%s server pid (docker) %d is ready", serverName, cmd.Process.Pid)
			return true
		case <-time.After(timeout):
			patternScanner.PrintLogs(t)
			// please don't use t.Fatalf() here as we could test if it failed later
			t.Errorf("failed to start %s server", serverName)
			return false
		}
	}
}

func RunHostServer(t *testing.T, command []string, env []string, serverStartRegex *regexp.Regexp) {
	if len(command) < 1 {
		t.Fatalf("command not set %v host server", command)
	}
	t.Helper()

	cmd := exec.Command(command[0], command[1:]...)
	serverName := cmd.String()
	patternScanner := NewScanner(serverStartRegex, make(chan struct{}, 1))

	cmd.Stdout = patternScanner
	cmd.Stderr = patternScanner
	cmd.Env = append(cmd.Env, env...)
	go func() {
		require.NoErrorf(t, cmd.Run(), "could not start %s on host", serverName)
	}()

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Process.Release()
	})

	for {
		select {
		case <-patternScanner.DoneChan:
			t.Logf("%s host server is ready", serverName)
			patternScanner.PrintLogs(t)
			return
		case <-time.After(time.Second * 60):
			patternScanner.PrintLogs(t)
			t.Fatalf("failed to start %s host server", serverName)
			return
		}
	}
}
