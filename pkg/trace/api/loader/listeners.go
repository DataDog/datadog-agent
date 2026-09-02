// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loader

import (
	"fmt"
	"net"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// GetUnixListener returns a net.Listener listening on the given "unix" socket path.
func GetUnixListener(path string) (net.Listener, error) {
	ln, err := filesystem.ListenUnix(path)
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
