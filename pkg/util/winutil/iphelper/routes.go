// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.
//go:build windows

package iphelper

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

//revive:disable:var-naming Name is intended to match the Windows API name
var (
	modiphelper = windows.NewLazyDLL("Iphlpapi.dll")

	procGetExtendedTcpTable = modiphelper.NewProc("GetExtendedTcpTable")
	procGetIpForwardTable   = modiphelper.NewProc("GetIpForwardTable")
	procGetIfTable          = modiphelper.NewProc("GetIfTable")
)

//revive:enable:var-naming (API)

//revive:disable:var-naming Name is intended to match the Windows type name

// MIB_TCPROW_OWNER_PID is the matching structure for the IPHelper structure
// of the same name. Fields documented
// https://docs.microsoft.com/en-us/windows/win32/api/tcpmib/ns-tcpmib-mib_tcprow_owner_pid
type MIB_TCPROW_OWNER_PID struct {
	/*  C declaration
	DWORD       dwState;
	DWORD       dwLocalAddr;
	DWORD       dwLocalPort;
	DWORD       dwRemoteAddr;
	DWORD       dwRemotePort;
	DWORD       dwOwningPid; */
	DwState      uint32
	DwLocalAddr  uint32 // network byte order
	DwLocalPort  uint32 // network byte order
	DwRemoteAddr uint32 // network byte order
	DwRemotePort uint32 // network byte order
	DwOwningPid  uint32
}

// MIB_IPFORWARDROW is the matching structure for the IPHelper structure of
// the same name; it defines a route entry
// https://docs.microsoft.com/en-us/windows/win32/api/ipmib/ns-ipmib-mib_ipforwardrow
type MIB_IPFORWARDROW struct {
	DwForwardDest      uint32 // destination IP address.  0.0.0.0 is default route
	DwForwardMask      uint32
	DwForwardPolicy    uint32
	DwForwardNextHop   uint32
	DwForwardIfIndex   uint32
	DwForwardType      uint32
	DwForwardProto     uint32
	DwForwardAge       uint32
	DwForwardNextHopAS uint32
	DwForwardMetric1   uint32
	DwForwardMetric2   uint32
	DwForwardMetric3   uint32
	DwForwardMetric4   uint32
	DwForwardMetric5   uint32
}

//revive:enable:var-naming (type)

//revive:disable:var-naming Name is intended to match the Windows const name

// TCP_TABLE_CLASS enum
//
// https://learn.microsoft.com/en-us/windows/win32/api/iprtrmib/ne-iprtrmib-tcp_table_class
const (
	TCP_TABLE_BASIC_LISTENER           = uint32(0)
	TCP_TABLE_BASIC_CONNECTIONS        = uint32(1)
	TCP_TABLE_BASIC_ALL                = uint32(2)
	TCP_TABLE_OWNER_PID_LISTENER       = uint32(3)
	TCP_TABLE_OWNER_PID_CONNECTIONS    = uint32(4)
	TCP_TABLE_OWNER_PID_ALL            = uint32(5)
	TCP_TABLE_OWNER_MODULE_LISTENER    = uint32(6)
	TCP_TABLE_OWNER_MODULE_CONNECTIONS = uint32(7)
	TCP_TABLE_OWNER_MODULE_ALL         = uint32(8)
)

//revive:enable:var-naming (const)

// GetIPv4RouteTable returns a list of the current ipv4 routes.
//
// https://learn.microsoft.com/en-us/windows/win32/api/iphlpapi/nf-iphlpapi-getipforwardtable
func GetIPv4RouteTable() (table []MIB_IPFORWARDROW, err error) {
	var size uint32
	var rawtableentry uintptr
	r, _, _ := procGetIpForwardTable.Call(rawtableentry,
		uintptr(unsafe.Pointer(&size)),
		uintptr(1)) // true, sorted

	if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
		err = fmt.Errorf("Unexpected error %v", r)
		return
	}
	rawbuf := make([]byte, size)
	r, _, _ = procGetIpForwardTable.Call(uintptr(unsafe.Pointer(&rawbuf[0])),
		uintptr(unsafe.Pointer(&size)),
		uintptr(1)) // true, sorted
	if r != 0 {
		err = fmt.Errorf("Unexpected error %v", r)
		return
	}
	count := uint32(binary.LittleEndian.Uint32(rawbuf))

	entries := (*[1 << 24]MIB_IPFORWARDROW)(unsafe.Pointer(&rawbuf[4]))[:count:count]
	table = append(table, entries...)
	return table, nil

}

// GetExtendedTcpV4Table returns a list of ipv4 tcp connections indexed by owning PID
//
// https://learn.microsoft.com/en-us/windows/win32/api/iphlpapi/nf-iphlpapi-getextendedtcptable
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func GetExtendedTcpV4Table() (table map[uint32][]MIB_TCPROW_OWNER_PID, err error) {
	var size uint32
	var rawtableentry uintptr
	r, _, _ := procGetExtendedTcpTable.Call(rawtableentry,
		uintptr(unsafe.Pointer(&size)),
		uintptr(0), // false, unsorted
		uintptr(syscall.AF_INET),
		uintptr(TCP_TABLE_OWNER_PID_ALL),
		uintptr(0))

	if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
		err = fmt.Errorf("Unexpected error %v", r)
		return
	}
	rawbuf := make([]byte, size)
	r, _, _ = procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&rawbuf[0])),
		uintptr(unsafe.Pointer(&size)),
		uintptr(0), // false, unsorted
		uintptr(syscall.AF_INET),
		uintptr(TCP_TABLE_OWNER_PID_ALL),
		uintptr(0))
	if r != 0 {
		err = fmt.Errorf("Unexpected error %v", r)
		return
	}
	count := uint32(binary.LittleEndian.Uint32(rawbuf))
	table = make(map[uint32][]MIB_TCPROW_OWNER_PID)

	entries := (*[1 << 24]MIB_TCPROW_OWNER_PID)(unsafe.Pointer(&rawbuf[4]))[:count:count]
	for _, entry := range entries {
		pid := entry.DwOwningPid

		table[pid] = append(table[pid], entry)

	}
	return table, nil

}

// GetIFTable returns a table of interfaces, indexed by the interface index
//
// https://learn.microsoft.com/en-us/windows/win32/api/iphlpapi/nf-iphlpapi-getiftable
func GetIFTable() (table map[uint32]windows.MibIfRow, err error) {
	var size uint32
	var rawtableentry uintptr
	r, _, _ := procGetIfTable.Call(rawtableentry,
		uintptr(unsafe.Pointer(&size)),
		uintptr(0)) // false, unsorted

	if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
		err = fmt.Errorf("Unexpected error %v", r)
		return
	}
	rawbuf := make([]byte, size)
	r, _, _ = procGetIfTable.Call(uintptr(unsafe.Pointer(&rawbuf[0])),
		uintptr(unsafe.Pointer(&size)),
		uintptr(0)) // false, unsorted
	if r != 0 {
		err = fmt.Errorf("Unexpected error %v", r)
		return
	}
	count := uint32(binary.LittleEndian.Uint32(rawbuf))
	table = make(map[uint32]windows.MibIfRow)

	entries := (*[1 << 20]windows.MibIfRow)(unsafe.Pointer(&rawbuf[4]))[:count:count]
	for _, entry := range entries {
		idx := entry.Index

		table[idx] = entry

	}
	return table, nil

}

// Ntohs converts a network byte order 16 bit int to host byte order
func Ntohs(i uint16) uint16 {
	return binary.BigEndian.Uint16((*(*[2]byte)(unsafe.Pointer(&i)))[:])
}

// Ntohl converts a network byte order 32 bit int to host byte order
func Ntohl(i uint32) uint32 {
	return binary.BigEndian.Uint32((*(*[4]byte)(unsafe.Pointer(&i)))[:])
}

// Htonl converts a host byte order 32 bit int to network byte order
func Htonl(i uint32) uint32 {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, i)
	return *(*uint32)(unsafe.Pointer(&b[0]))
}
