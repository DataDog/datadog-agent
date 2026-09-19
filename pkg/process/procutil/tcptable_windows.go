// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procutil

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"net"
	"unsafe"

	"golang.org/x/sys/cpu"
	"golang.org/x/sys/windows"
)

// tcpTableOwnerPIDAll is TCP_TABLE_OWNER_PID_ALL: one row per connection tagged with its owning PID.
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

// portFromField decodes a port stored in network byte order within the low 16 bits of a 32-bit field.
func portFromField(v uint32) uint16 {
	if !cpu.IsBigEndian {
		return uint16(bits.ReverseBytes32(v) >> 16)
	}
	return uint16(v >> 16)
}

// addrFromV4Field decodes an IPv4 address occupying the full 32-bit field, re-serializing with the same byte order to reconstruct the network-order bytes.
func addrFromV4Field(v uint32) net.IP {
	buf := make([]byte, 4)
	if cpu.IsBigEndian {
		binary.BigEndian.PutUint32(buf, v)
	} else {
		binary.LittleEndian.PutUint32(buf, v)
	}
	return net.IP(buf)
}

// getExtendedTCPTable calls GetExtendedTcpTable for the family, growing the buffer until the kernel-reported size fits.
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

// findConnectionOwnerPID returns the PID owning the given connection; bool is false with nil error when none matches, non-nil error means the table couldn't be read.
func findConnectionOwnerPID(family uint32, localAddr net.IP, localPort int, remoteAddr net.IP, remotePort int) (uint32, bool, error) {
	table, err := getExtendedTCPTable(family)
	if err != nil {
		return 0, false, fmt.Errorf("failed to read the %s TCP table: %w", tcpFamilyName(family), err)
	}
	if family == windows.AF_INET6 {
		pid, ok := findPIDInV6Table(table, localPort, remotePort, localAddr, remoteAddr)
		return pid, ok, nil
	}
	pid, ok := findPIDInV4Table(table, localPort, remotePort, localAddr, remoteAddr)
	return pid, ok, nil
}

func tcpFamilyName(family uint32) string {
	if family == windows.AF_INET6 {
		return "IPv6"
	}
	return "IPv4"
}
