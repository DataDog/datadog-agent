// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.
//go:build windows

package iphelper

import (
	"C"

	"fmt"
	"net"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetAdaptersAddresses = modiphelper.NewProc("GetAdaptersAddresses")
)

type IPAdapterUnicastAddress struct {
	Flags   uint32
	Address net.IP
}

type sockaddr struct {
	family int16
	port   uint16
	// if it's ipv4, the address is the first 4 bytes
	// if it's ipv6, the address is bytes 4->20
	addressBase uintptr
}
type socketAddress struct {
	lpSockaddr      *sockaddr
	iSockaddrLength int32
}
type ipAdapterUnicastAddress struct {
	length  uint32
	flags   uint32
	next    *ipAdapterUnicastAddress
	address socketAddress
}

// IpAdapterAddressesLh is a go adaptation of the C structure IP_ADAPTER_ADDRESSES_LH
// it is a go adaptation, rather than a matching structure, because the real structure
// is difficult to approximate in Go.
type IpAdapterAddressesLh struct {
	Index            uint32
	AdapterName      string
	UnicastAddresses []IPAdapterUnicastAddress
}

type ipAdapterAddresses struct {
	length              uint32
	ifIndex             uint32
	next                *ipAdapterAddresses
	adapterName         unsafe.Pointer // pointer to character buffer
	firstUnicastAddress *ipAdapterUnicastAddress
}

// GetAdaptersAddresses returns a map of all of the adapters, indexed by
// interface index
func GetAdaptersAddresses() (table map[uint32]IpAdapterAddressesLh, err error) {
	size := uint32(15 * 1024)
	rawbuf := make([]byte, size)

	r, _, _ := procGetAdaptersAddresses.Call(uintptr(syscall.AF_INET),
		uintptr(0), // flags == 0 for now
		uintptr(0), // reserved, always zero
		uintptr(unsafe.Pointer(&rawbuf[0])),
		uintptr(unsafe.Pointer(&size)))

	if r != 0 {
		if r != uintptr(windows.ERROR_BUFFER_OVERFLOW) {
			err = fmt.Errorf("Error getting address list %v", r)
			return
		}
		rawbuf = make([]byte, size)
		r, _, _ := procGetAdaptersAddresses.Call(uintptr(syscall.AF_INET),
			uintptr(0), // flags == 0 for now
			uintptr(0), // reserved, always zero
			uintptr(unsafe.Pointer(&rawbuf[0])),
			uintptr(unsafe.Pointer(&size)))
		if r != 0 {
			err = fmt.Errorf("Error getting address list %v", r)
			return
		}
	}
	// need to walk the list.  The list is a C style list.  The `Next` pointer
	// is not the first element.  The C structure is as follows
	/*
		typedef struct _IP_ADAPTER_ADDRESSES_LH {
			union {
				ULONGLONG Alignment;
				struct {
				ULONG    Length;
				IF_INDEX IfIndex;
				};
			};
			struct _IP_ADAPTER_ADDRESSES_LH    *Next;
			PCHAR                              AdapterName;
			PIP_ADAPTER_UNICAST_ADDRESS_LH     FirstUnicastAddress;
			// more fields follow which we're not using
	*/
	var addr *ipAdapterAddresses
	table = make(map[uint32]IpAdapterAddressesLh)
	addr = (*ipAdapterAddresses)(unsafe.Pointer(&rawbuf[0]))
	for addr != nil {
		var entry IpAdapterAddressesLh
		entry.Index = addr.ifIndex
		entry.AdapterName = C.GoString((*C.char)(addr.adapterName))

		unicast := addr.firstUnicastAddress
		for unicast != nil {
			if unicast.address.lpSockaddr.family == syscall.AF_INET {
				// ipv4 address
				var uni IPAdapterUnicastAddress
				uni.Address = (*[1 << 29]byte)(unsafe.Pointer(unicast.address.lpSockaddr))[4:8:8]
				entry.UnicastAddresses = append(entry.UnicastAddresses, uni)
			} else if unicast.address.lpSockaddr.family == syscall.AF_INET6 {
				var uni IPAdapterUnicastAddress
				uni.Address = (*[1 << 29]byte)(unsafe.Pointer(&(unicast.address.lpSockaddr.addressBase)))[:16:16]
				entry.UnicastAddresses = append(entry.UnicastAddresses, uni)
			}
			unicast = unicast.next
		}
		table[entry.Index] = entry
		addr = addr.next
	}
	return
}
