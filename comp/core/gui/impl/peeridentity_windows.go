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

// resolveOwnerSIDTimeout caps how long a stalled process-agent can delay minting/redeeming; a var so tests can shrink it.
var resolveOwnerSIDTimeout = 2 * time.Second

var (
	// processAgentIPC/processAgentConfig back resolveConnectionOwnerSID; set once by configurePeerIdentityResolution before the listener starts, so no sync needed.
	processAgentIPC    ipc.Component
	processAgentConfig pkgconfigmodel.Reader
)

// configurePeerIdentityResolution records the IPC client and config resolveConnectionOwnerSID needs to call process-agent.
func configurePeerIdentityResolution(ipcComp ipc.Component, cfg pkgconfigmodel.Reader) {
	processAgentIPC = ipcComp
	processAgentConfig = cfg
}

// lookupLoopbackPeerIdentity asks process-agent (LocalSystem) to attribute the loopback connection's 4-tuple to a process and read its token SID; sending the 4-tuple (not a PID) closes the PID-reuse race (see pkg/process/procutil.GetSIDForConnectionOwner). serverAddr/serverPort is the GUI listener; peerAddr/peerPort is the caller.
func lookupLoopbackPeerIdentity(serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	family := "6"
	if peerAddr.To4() != nil {
		family = "4"
	}
	return resolveConnectionOwnerSID(family, peerAddr, peerPort, serverAddr, serverPort)
}

// elevatedMintIdentity is effectively unreachable here: peerIdentity is a SID string, never "0", so mintTimeIdentity's root check never fires on Windows.
func elevatedMintIdentity() peerIdentity {
	return rootIdentity
}

// resolveConnectionOwnerSID asks process-agent (LocalSystem, which holds SeDebugPrivilege) for the SID owning the loopback connection via GET /connection/owner-sid over the IPC mTLS client, so ddagentuser needs no SeDebugPrivilege (see cmd/process-agent/api/connection_windows.go).
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
