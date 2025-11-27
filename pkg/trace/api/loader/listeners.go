// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loader

import (
	"fmt"
	"net"
	"os"
)

// GetUnixListener returns a net.Listener listening on the given "unix" socket path.
func GetUnixListener(path string) (net.Listener, error) {
	fi, err := os.Stat(path)
	if err == nil {
		// already exists
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("cannot reuse %q; not a unix socket", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("unable to remove stale socket: %v", err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if unixLn, ok := ln.(*net.UnixListener); ok {
		// We do not want to unlink the socket here as we can't be sure if another trace-agent has already
		// put a new file at the same path.
		unixLn.SetUnlinkOnClose(false)
	}
	if err := os.Chmod(path, 0o722); err != nil {
		return nil, fmt.Errorf("error setting socket permissions: %v", err)
	}
	return ln, nil
}

// GetTCPListener returns a net.Listener listening on the given TCP address.
func GetTCPListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}
