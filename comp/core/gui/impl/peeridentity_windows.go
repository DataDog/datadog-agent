// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package guiimpl

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"net"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/cpu"
	"golang.org/x/sys/windows"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// sidForPIDTimeout bounds sidForPID's call to process-agent: it caps both the PID-reuse race window (the gap between the GUI reading a PID off the TCP table and process-agent resolving that PID's SID) and how long a stalled or overloaded process-agent can delay the caller. A var, not a const, so tests can shrink it instead of waiting out the real duration.
var sidForPIDTimeout = 2 * time.Second

var (
	// processAgentIPC and processAgentConfig back sidForPID below, which asks process-agent for a PID's owning SID instead of opening the process's token directly. Set once by configurePeerIdentityResolution, called from NewComponent before the GUI's HTTP listener starts, so no synchronization is needed between that write and sidForPID's later reads.
	processAgentIPC    ipc.Component
	processAgentConfig pkgconfigmodel.Reader
)

// configurePeerIdentityResolution records the agent-wide IPC client and config that sidForPID needs to call process-agent.
func configurePeerIdentityResolution(ipcComp ipc.Component, cfg pkgconfigmodel.Reader) {
	processAgentIPC = ipcComp
	processAgentConfig = cfg
}

// tcpTableOwnerPIDAll requests TCP_TABLE_OWNER_PID_ALL from GetExtendedTcpTable: one row per connection, each tagged with its owning PID.
const tcpTableOwnerPIDAll = 5

var (
	iphlpapiDLL             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTCPTable = iphlpapiDLL.NewProc("GetExtendedTcpTable")
)

type mibTCPRowOwnerPID struct {
	state      uint32
	localAddr  uint32
	localPort  uint32
	remoteAddr uint32
	remotePort uint32
	pid        uint32
}

type mibTCPTableOwnerPID struct {
	numEntries uint32
	table      [1]mibTCPRowOwnerPID
}

type mibTCP6RowOwnerPID struct {
	localAddr   [16]byte
	localScope  uint32
	localPort   uint32
	remoteAddr  [16]byte
	remoteScope uint32
	remotePort  uint32
	state       uint32
	pid         uint32
}

type mibTCP6TableOwnerPID struct {
	numEntries uint32
	table      [1]mibTCP6RowOwnerPID
}

// portFromField decodes a port number stored, like the other fields GetExtendedTcpTable returns, in network byte order within the low 16 bits of a 32-bit field.
func portFromField(v uint32) uint16 {
	if !cpu.IsBigEndian {
		return uint16(bits.ReverseBytes32(v) >> 16)
	}
	return uint16(v >> 16)
}

// addrFromV4Field decodes an IPv4 address from a MIB_TCPROW_OWNER_PID field; unlike the port fields, it occupies the full 32 bits, so re-serializing with the same byte order it was read with reconstructs the original network-order bytes.
func addrFromV4Field(v uint32) net.IP {
	buf := make([]byte, 4)
	if cpu.IsBigEndian {
		binary.BigEndian.PutUint32(buf, v)
	} else {
		binary.LittleEndian.PutUint32(buf, v)
	}
	return net.IP(buf)
}

// getExtendedTCPTable calls GetExtendedTcpTable for the given address family, growing the buffer until the kernel-reported size is satisfied.
func getExtendedTCPTable(family uint32) ([]byte, error) {
	var size uint32
	var buf []byte
	for {
		var addr unsafe.Pointer
		if len(buf) > 0 {
			addr = unsafe.Pointer(&buf[0])
		}
		ret, _, _ := procGetExtendedTCPTable.Call(
			uintptr(addr),
			uintptr(unsafe.Pointer(&size)),
			1, // sorted
			uintptr(family),
			tcpTableOwnerPIDAll,
			0,
		)
		if ret == 0 {
			return buf[:size], nil
		}
		if windows.Errno(ret) != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil, windows.Errno(ret)
		}
		const maxTableSize = 10 << 20
		if size < 4 || size > maxTableSize {
			return nil, fmt.Errorf("unreasonable table size %d reported by GetExtendedTcpTable", size)
		}
		buf = make([]byte, size)
	}
}

func findPIDInV4Table(buf []byte, localPort, remotePort int, localAddr, remoteAddr net.IP) (uint32, bool) {
	if len(buf) == 0 {
		return 0, false
	}
	info := (*mibTCPTableOwnerPID)(unsafe.Pointer(&buf[0]))
	rows := unsafe.Slice(&info.table[0], info.numEntries)
	for i := range rows {
		if int(portFromField(rows[i].localPort)) == localPort &&
			int(portFromField(rows[i].remotePort)) == remotePort &&
			addrFromV4Field(rows[i].localAddr).Equal(localAddr) &&
			addrFromV4Field(rows[i].remoteAddr).Equal(remoteAddr) {
			return rows[i].pid, true
		}
	}
	return 0, false
}

func findPIDInV6Table(buf []byte, localPort, remotePort int, localAddr, remoteAddr net.IP) (uint32, bool) {
	if len(buf) == 0 {
		return 0, false
	}
	info := (*mibTCP6TableOwnerPID)(unsafe.Pointer(&buf[0]))
	rows := unsafe.Slice(&info.table[0], info.numEntries)
	for i := range rows {
		if int(portFromField(rows[i].localPort)) == localPort &&
			int(portFromField(rows[i].remotePort)) == remotePort &&
			net.IP(rows[i].localAddr[:]).Equal(localAddr) &&
			net.IP(rows[i].remoteAddr[:]).Equal(remoteAddr) {
			return rows[i].pid, true
		}
	}
	return 0, false
}

// lookupLoopbackPeerIdentity finds the SID owning the loopback TCP connection; peerAddr's own family (not a IPv4-then-IPv6 fallback) decides which table to query, since two connections could otherwise share a port pair across families and misattribute the SID.
func lookupLoopbackPeerIdentity(serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	if v4 := peerAddr.To4(); v4 != nil {
		table, err := getExtendedTCPTable(windows.AF_INET)
		if err != nil {
			return "", fmt.Errorf("failed to get IPv4 TCP table: %w", err)
		}
		if pid, ok := findPIDInV4Table(table, peerPort, serverPort, v4, serverAddr); ok {
			return sidForPID(pid)
		}
		return "", fmt.Errorf("no matching IPv4 TCP connection for local port %d, remote port %d", peerPort, serverPort)
	}

	table, err := getExtendedTCPTable(windows.AF_INET6)
	if err != nil {
		return "", fmt.Errorf("failed to get IPv6 TCP table: %w", err)
	}
	if pid, ok := findPIDInV6Table(table, peerPort, serverPort, peerAddr, serverAddr); ok {
		return sidForPID(pid)
	}
	return "", fmt.Errorf("no matching IPv6 TCP connection for local port %d, remote port %d", peerPort, serverPort)
}

// elevatedMintIdentity is unreachable here: peerIdentity is a Windows SID string, never a plain "0" (see rootIdentity), so mintTimeIdentity's root check never triggers on this platform.
func elevatedMintIdentity() peerIdentity {
	return rootIdentity
}

// sidForPID asks process-agent for pid's owning SID over the agent-wide IPC mTLS client, rather than opening the process's token directly from this lower-privileged ddagentuser service. process-agent runs as LocalSystem (which already holds SeDebugPrivilege by default) and exposes GET /pid/{pid}/sid for exactly this lookup (see pkg/process/procutil.GetSIDForPID and cmd/process-agent/api/pid_windows.go), so ddagentuser no longer needs SeDebugPrivilege granted to it directly.
func sidForPID(pid uint32) (peerIdentity, error) {
	if processAgentIPC == nil || processAgentConfig == nil {
		return "", errors.New("peer identity resolution is not configured")
	}

	addrPort, err := pkgconfighelper.GetProcessAPIAddressPort(processAgentConfig)
	if err != nil {
		return "", fmt.Errorf("failed to resolve process-agent address: %w", err)
	}

	url := fmt.Sprintf("https://%s/pid/%d/sid", addrPort, pid)
	body, err := processAgentIPC.GetClient().Get(url, ipchttp.WithLeaveConnectionOpen, ipchttp.WithTimeout(sidForPIDTimeout))
	if err != nil {
		return "", fmt.Errorf("failed to query process-agent for pid %d's SID: %w", pid, err)
	}

	sid := strings.TrimSpace(string(body))
	if sid == "" {
		return "", fmt.Errorf("process-agent returned an empty SID for pid %d", pid)
	}

	return peerIdentity(sid), nil
}
