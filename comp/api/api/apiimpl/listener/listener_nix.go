// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package listener

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// platformSpecificListener returns a unix net.Listener for linux platforms
func platformSpecificListener(address string) (net.Listener, error) {
	fi, err := os.Stat(address)
	if err == nil {
		// already exists
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("cannot reuse %q; not a unix socket", address)
		}
		if err := os.Remove(address); err != nil {
			return nil, fmt.Errorf("unable to remove stale socket: %v", err)
		}
	}

	ln, err := net.Listen("unix", address)
	if err != nil {
		return nil, err
	}

	// 0770 so non-root processes in the dd-agent group can connect
	// (connecting to a unix socket needs write). os.Root.Chmod/Chown act
	// on the path entry and refuse to follow symlinks, closing a TOCTOU
	// where the socket is swapped for a symlink between Listen and here.
	root, err := os.OpenRoot(filepath.Dir(address))
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("unable to open run directory for socket %s: %w", address, err)
	}
	defer root.Close()

	name := filepath.Base(address)
	if err := root.Chmod(name, 0770); err != nil {
		ln.Close()
		return nil, fmt.Errorf("unable to set permissions on socket %s: %w", address, err)
	}

	// When root, hand ownership to dd-agent so non-root agent processes
	// can connect. Root is unaffected by owner.
	if os.Geteuid() == 0 {
		if uid, gid, ok := filesystem.GetAgentUserGroupIDs(); ok {
			if err := root.Chown(name, uid, gid); err != nil {
				log.Warnf("unable to set dd-agent ownership for socket %s: %v", address, err)
			} else {
				log.Debugf("set socket ownership to dd-agent (uid=%d, gid=%d): %s", uid, gid, address)
			}
		}
	}

	return ln, nil
}

func hasPlatformSupport() bool {
	return true
}
