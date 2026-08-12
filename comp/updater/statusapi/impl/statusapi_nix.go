// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package statusapiimpl

import (
	"fmt"
	"net"
	"os"

	statusapi "github.com/DataDog/datadog-agent/comp/updater/statusapi/def"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// socketMode gives the owning group — the Agent user's group — the write permission
// connecting to a unix socket requires, and nothing to anyone else. Same mode as
// system-probe's socket (pkg/system-probe/api/server/listener_unix.go).
const socketMode = 0720

// listen binds the status socket and opens it to the Agent user's group.
//
// The daemon runs as root while the Agent runs as dd-agent, so — unlike the local
// API's 0700 socket — this one has to be reachable by the Agent user.
func listen() (net.Listener, error) {
	return listenStatusSocket(statusapi.Endpoint())
}

// listenStatusSocket takes the path so tests do not have to write to paths.RunPath.
//
// paths.RunPath is owned by dd-agent and mode 0755 (see the run directory in
// pkg/fleet/installer/packages/datadog_agent_linux.go), so every step below has to
// assume an unprivileged process can create, replace or unlink entries in that
// directory while we work. That is what makes this different from system-probe,
// whose otherwise identical recipe operates in a root-owned directory:
//
//   - Whatever sits at the path is removed, socket or not. Refusing to unlink a
//     non-socket would let any dd-agent process keep the daemon from starting by
//     planting a regular file there.
//   - The mode is set through the umask at bind time instead of with a chmod
//     afterwards, since chmod takes a path — see filesystem.ListenUnixSocketWithMode.
//   - The group is set with Lchown, which does not follow symlinks, and only the
//     group changes so the socket stays owned by root.
//
// The alternative that would remove the umask swap altogether is to move the socket
// into a directory dd-agent cannot write to, which makes the plain chmod recipe safe
// again. That is a packaging change (and a change to a path we are about to
// document), so it is deliberately not done here.
func listenStatusSocket(socketPath string) (net.Listener, error) {
	if err := os.RemoveAll(socketPath); err != nil {
		return nil, fmt.Errorf("could not remove existing status socket: %w", err)
	}

	listener, err := filesystem.ListenUnixSocketWithMode(socketPath, socketMode)
	if err != nil {
		return nil, err
	}

	// A no-op when the Agent user does not exist, which is the right behaviour on a
	// host running the installer without an Agent.
	if err := filesystem.SetAgentGroupOwnerNoFollow(socketPath); err != nil {
		listener.Close()
		return nil, fmt.Errorf("error setting status socket group: %w", err)
	}

	return listener, nil
}
