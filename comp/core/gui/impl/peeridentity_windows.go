// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package guiimpl

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// resolveOwnerSIDTimeout bounds the call to process-agent: it caps how long a stalled or overloaded
// process-agent can delay minting or redeeming. A var, not a const, so tests can shrink it instead of
// waiting out the real duration.
var resolveOwnerSIDTimeout = 2 * time.Second

var (
	// processAgentIPC and processAgentConfig back resolveConnectionOwnerSID below, which asks process-agent
	// to resolve a loopback connection's owning SID instead of opening the process's token directly. Set
	// once by configurePeerIdentityResolution, called from NewComponent before the GUI's HTTP listener
	// starts, so no synchronization is needed between that write and the later reads.
	processAgentIPC    ipc.Component
	processAgentConfig pkgconfigmodel.Reader
)

// configurePeerIdentityResolution records the agent-wide IPC client and config that resolveConnectionOwnerSID needs to call process-agent.
func configurePeerIdentityResolution(ipcComp ipc.Component, cfg pkgconfigmodel.Reader) {
	processAgentIPC = ipcComp
	processAgentConfig = cfg
}

// lookupLoopbackPeerIdentity resolves the SID owning the loopback TCP connection by asking process-agent,
// which runs as LocalSystem, to attribute the connection to a process and read its token SID. The
// connection's own 4-tuple is sent — not a PID this process resolved itself — because a PID is not a stable
// identifier: resolving it in process-agent, where the owning handle is held and ownership is re-validated,
// is what closes the PID-reuse race (see pkg/process/procutil.GetSIDForConnectionOwner). serverAddr/
// serverPort are the GUI listener's endpoint; peerAddr/peerPort are the connecting caller's, i.e. the local
// endpoint of the peer's own socket.
func lookupLoopbackPeerIdentity(serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	family := "6"
	if peerAddr.To4() != nil {
		family = "4"
	}
	return resolveConnectionOwnerSID(family, peerAddr, peerPort, serverAddr, serverPort)
}

// elevatedMintIdentity is unreachable here: peerIdentity is a Windows SID string, never a plain "0" (see rootIdentity), so mintTimeIdentity's root check never triggers on this platform.
func elevatedMintIdentity() peerIdentity {
	return rootIdentity
}

// resolveConnectionOwnerSID asks process-agent for the SID owning the loopback connection whose local
// endpoint is localAddr:localPort and whose remote endpoint is remoteAddr:remotePort, over the agent-wide
// IPC mTLS client. process-agent runs as LocalSystem (which already holds SeDebugPrivilege by default) and
// exposes GET /connection/owner-sid for exactly this lookup (see pkg/process/procutil.GetSIDForConnectionOwner
// and cmd/process-agent/api/connection_windows.go), so ddagentuser no longer needs SeDebugPrivilege granted
// to it directly.
func resolveConnectionOwnerSID(family string, localAddr net.IP, localPort int, remoteAddr net.IP, remotePort int) (peerIdentity, error) {
	if processAgentIPC == nil || processAgentConfig == nil {
		return "", errors.New("peer identity resolution is not configured")
	}

	addrPort, err := pkgconfighelper.GetProcessAPIAddressPort(processAgentConfig)
	if err != nil {
		return "", fmt.Errorf("failed to resolve process-agent address: %w", err)
	}

	query := url.Values{
		"family": {family},
		"laddr":  {localAddr.String()},
		"lport":  {strconv.Itoa(localPort)},
		"raddr":  {remoteAddr.String()},
		"rport":  {strconv.Itoa(remotePort)},
	}
	requestURL := fmt.Sprintf("https://%s/connection/owner-sid?%s", addrPort, query.Encode())

	body, err := processAgentIPC.GetClient().Get(requestURL, ipchttp.WithLeaveConnectionOpen, ipchttp.WithTimeout(resolveOwnerSIDTimeout))
	if err != nil {
		return "", fmt.Errorf("failed to query process-agent for the connection owner's SID: %w", err)
	}

	sid := strings.TrimSpace(string(body))
	if sid == "" {
		return "", errors.New("process-agent returned an empty SID")
	}

	return peerIdentity(sid), nil
}
