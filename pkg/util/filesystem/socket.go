// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package filesystem

import (
	"fmt"
	"net"
	"os"
)

// ListenUnix returns a listener for the given socket path, after removing any stale socket.
func ListenUnix(socketPath string) (net.Listener, error) {
	// TODO: loop until the socket is actually removed, to avoid race conditions
	// TODO: net.ListenConfig with Control function to set socket owner/permissions before binding
	// TODO: don't follow symlinks when Removing
	// TODO: fix race between os.Stat and os.Remove
	// TODO: check if the socket is actually stale or still in use
	if err := removeStaleSocket(socketPath); err != nil {
		return nil, err
	}

	return net.Listen("unix", socketPath)
}

func removeStaleSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat existing file %q: %w", socketPath, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to bind UDS at %q: path exists and is not a socket (%s)", socketPath, info.Mode().String())
	}
	if err := os.Remove(socketPath); err != nil {
		return fmt.Errorf("failed to remove stale UDS at %q: %w", socketPath, err)
	}
	return nil
}
