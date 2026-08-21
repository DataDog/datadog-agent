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
	"os/user"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/unix"
)

// platformSpecificListener returns a unix net.Listener for linux platforms
func platformSpecificListener(address string) (net.Listener, error) {
	ln, err := net.Listen("unix", address)
	if err != nil {
		return nil, err
	}

	// Restrict access to the agent user and its group. Other agent unix
	// sockets (statsd, apm, the runtime security agent) already lock down
	// their permissions; the IPC/CMD sockets defaulted to 0755, which
	// prevented non-root subagents (e.g. the host profiler) from
	// connecting since connecting to a unix socket requires write access.
	//
	// The default run directory is writable by dd-agent, so a compromised
	// dd-agent process could rename the freshly-created socket and replace
	// it with a symlink between net.Listen and a path-based chmod/chown,
	// redirecting those calls at an arbitrary root-owned file. Use the
	// *at syscalls with AT_SYMLINK_NOFOLLOW so the calls act on the path
	// entry itself and refuse to follow symlinks, closing that TOCTOU
	// window. fchmodat2 (Linux 5.6+) honors the flag; on older kernels
	// without it, unix.Fchmodat returns EOPNOTSUPP instead of following
	// the symlink, so it is safe (the socket simply keeps its default
	// mode and non-root subagents cannot connect).
	if err := unix.Fchmodat(unix.AT_FDCWD, address, 0770, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if err == unix.EOPNOTSUPP || err == unix.ENOTSUP {
			log.Warnf("kernel does not support chmod without following symlinks, leaving socket %s with default permissions: %v", address, err)
		} else {
			ln.Close()
			return nil, fmt.Errorf("unable to set permissions on socket %s: %w", address, err)
		}
	}

	// When running as root, hand ownership to the dd-agent user/group when
	// it exists so that non-root agent processes running as dd-agent can
	// connect to the socket. Root can still access it regardless of owner.
	if os.Geteuid() == 0 {
		if uid, gid, ok := ddAgentIDs(); ok {
			if err := unix.Fchownat(unix.AT_FDCWD, address, uid, gid, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				log.Warnf("unable to set dd-agent ownership for socket %s: %v", address, err)
			} else {
				log.Debugf("set socket ownership to dd-agent (uid=%d, gid=%d): %s", uid, gid, address)
			}
		}
	}

	return ln, nil
}

// ddAgentIDs returns the UID and GID of the dd-agent user and group when both
// exist. It returns ok=false if either lookup fails.
func ddAgentIDs() (uid, gid int, ok bool) {
	u, err := user.Lookup("dd-agent")
	if err != nil {
		return 0, 0, false
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, false
	}
	g, err := user.LookupGroup("dd-agent")
	if err != nil {
		return 0, 0, false
	}
	gid, err = strconv.Atoi(g.Gid)
	if err != nil {
		return 0, 0, false
	}
	return uid, gid, true
}

func hasPlatformSupport() bool {
	return true
}
