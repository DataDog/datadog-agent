// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package preflightmodeimpl

import (
	"fmt"
	"os"
	"path/filepath"
)

// maxUnixSocketPath bounds the socket path length. sockaddr_un.sun_path is 108 bytes on
// Linux and 104 on macOS, including the terminator; the bind fails with a bare
// "invalid argument" when it is exceeded, which is near-impossible to diagnose from
// telemetry. 100 is conservative for both.
const maxUnixSocketPath = 100

// listener describes the throwaway DogStatsD endpoint the preflight ADP binds. On Unix it is
// a datagram unix socket inside the working directory, so it is unreachable from off-host
// and disappears with the directory.
type listener struct {
	socketPath string
}

// newListener builds the listener description for a working directory.
func newListener(workDir string) listener {
	return listener{socketPath: filepath.Join(workDir, "dsd.socket")}
}

// validate reports whether the endpoint can be bound at all, so that an unbindable path is
// reported as a clear failure instead of surfacing as an opaque bind error from ADP.
func (l listener) validate() error {
	if len(l.socketPath) > maxUnixSocketPath {
		return fmt.Errorf("the socket path is %d bytes, which exceeds the %d byte limit: %s",
			len(l.socketPath), maxUnixSocketPath, l.socketPath)
	}
	return nil
}

// configOverrides returns the DogStatsD listener settings for the generated ADP config.
func (l listener) configOverrides() map[string]any {
	return map[string]any{
		"dogstatsd_socket": l.socketPath,
		// Avoid any inherited value from causing ADP to try and bind to a Windows named pipe
		// on a non-Windows platform.
		"dogstatsd_pipe_name": "",
	}
}

// probeAddr is the address the probe's DogStatsD client dials.
func (l listener) probeAddr() string {
	return "unix://" + l.socketPath
}

// ready reports whether ADP has bound the endpoint yet.
func (l listener) ready() bool {
	fi, err := os.Stat(l.socketPath)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSocket != 0
}

// describe returns a human-readable form for logs.
func (l listener) describe() string {
	return l.socketPath
}
