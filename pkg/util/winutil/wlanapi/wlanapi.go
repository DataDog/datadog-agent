// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

// Package iisconfig manages iis configuration
package wlanapi

/*
the file datadog.json can be located anywhere; it is path-relative to a .net application
give the path name, read the json and return it as a map of string/string
*/

import (

	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modwlanapi = windows.NewLazyDLL("wlanapi.dll")
	procWLANOpenHandle = modwlanapi.NewProc("WlanOpenHandle")
	procWLANCloseHandle = modwlanapi.NewProc("WlanCloseHandle")
	procWLANEnumInterfaces = modwlanapi.NewProc("WlanEnumInterfaces")
	procWLANGetAvailableNetworkList = modwlanapi.NewProc("WlanGetAvailableNetworkList")
	procWLANFreeMemory = modwlanapi.NewProc("WlanFreeMemory")
)

const dwMaxClientVersion = int32(2) // Windows Vista and Windows Server 2008 (which is the max)

type WLANHandle struct {
	handle windows.Handle
}

// in C, interfaceInfoList is defined as
/*
typedef struct _WLAN_INTERFACE_INFO_LIST {
    DWORD dwNumberOfItems;
    DWORD dwIndex;

    WLAN_INTERFACE_INFO *InterfaceInfo

} WLAN_INTERFACE_INFO_LIST, *PWLAN_INTERFACE_INFO_LIST;
 */
type WLAN_INTERFACE_INFO_LIST struct {
	dwNumberOfItems uint32
	dwIndex         uint32
	interfaces      uintptr
}

type WLAN_INTERFACE_INFO struct {
	guid windows.GUID
	interfaceDescription [256]uint16  // in windows it's a widechar string, typed to max number of chars 256
	isState uint32
}

type interfaceInfo struct {
	guid windows.GUID
	interfaceDescription string
	isState uint32
}
type interfaceInfoList struct {
	interfaces []interfaceInfo
}

const (
	// taken from wlanapi.h.  Should probably be compile-time generated
	wlanInterfaceStateNotReady = 0
	wlanInterfaceStateConnected = 1
	wlanInterfaceStateAdHocNetworkFormed = 2
	wlanInterfaceStateDisconnecting = 3
	wlanInterfaceStateDisconnected = 4
	wlanInterfaceStateAssociating = 5
	wlanInterfaceStateDiscovering = 6
	wlanInterfaceStateAuthenticating = 7
)
func OpenWLANHandle() (*WLANHandle, error) {
	var handle windows.Handle
	var currVersion int32
	ret, _, _ := procWLANOpenHandle.Call(uintptr(dwMaxClientVersion), 0, uintptr(unsafe.Pointer(&currVersion)), uintptr(unsafe.Pointer(&handle)))
	if ret != 0 {
		return nil, fmt.Errorf("WlanOpenHandle failed with error code %d", ret)
	}
	return &WLANHandle{handle: handle}, nil
}
func (wh *WLANHandle) Close() {
	procWLANCloseHandle.Call(uintptr(wh.handle), 0)
}

func (wh *WLANHandle) enumInterfaces() (*interfaceInfoList, error) {
	var pInterfaceList uintptr
	ret, _, _ := procWLANEnumInterfaces.Call(uintptr(wh.handle), 0, uintptr(unsafe.Pointer(&pInterfaceList)))
	if ret != 0 {
		return nil, fmt.Errorf("WlanEnumInterfaces failed with error code %d", ret)
	}
	defer procWLANFreeMemory.Call(pInterfaceList)

	iflist := (*WLAN_INTERFACE_INFO_LIST)(unsafe.Pointer(pInterfaceList))
	ifaces := unsafe.Slice((*WLAN_INTERFACE_INFO)(unsafe.Pointer(&iflist.interfaces)), int(iflist.dwNumberOfItems))
	// once we free the memory (above), we can't use it any more.  So we can't just use the unsafe.Slice
	// we have to make a deep copy
	iil := interfaceInfoList {
		interfaces: make([]interfaceInfo, len(ifaces)),
	}
	for i, iface := range ifaces {
		iil.interfaces[i] = interfaceInfo {
			guid: iface.guid,
			interfaceDescription: windows.UTF16ToString(iface.interfaceDescription[:]),
			isState: iface.isState,
		}
	}
	return &iil, nil

}

type WLANNetwork struct {
	NetworkID string
	SSID string
	BssType uint32
	SignalStrength int32 // will always be negative, in dBm
	Connectable bool
	NotConnectableReason uint32
	Connected bool
	HasProfile bool

}
type WLANNetworkInterface struct {
	InterfaceIndex uint32
	InterfaceGUID windows.GUID
	InterfaceDescription string
	InterfaceState uint32
	Networks []WLANNetwork
}

type DOT11_SSID struct {
	len uint32
	ssid [32]uint8
}
type WLAN_AVAILABLE_NETWORK struct {
	networkID [256]uint16
	dot11Ssid DOT11_SSID
	dot11BssType uint32
	uNumberOfBssids uint32
	bNetworkConnectable bool
	wlanNotConnectableReason uint32
	uNumberOfPhyTypes uint32
	dot11PhyTypes [8]uint32
	bMorePhyTypes bool
	signalQuality int32
	securityEnabled bool
	dot11DefaultAuthAlgorithm uint32
	dot11DefaultCipherAlgorithm uint32
	flags uint32
	reserved uint32
}
type WLAN_AVAILABLE_NETWORK_LIST struct {
	dwNumberOfItems uint32
	dwIndex uint32
	networks *WLAN_AVAILABLE_NETWORK
}
func (wh *WLANHandle) EnumNetworks() ([]WLANNetworkInterface, error) {

	il, err := wh.enumInterfaces()
	if err != nil {
		return nil, err
	}
	if len(il.interfaces) == 0 {
		return nil, nil
	}
	ifaces := make([]WLANNetworkInterface, 0, len(il.interfaces))
	for _, iface := range il.interfaces {
		ni := WLANNetworkInterface {
			InterfaceGUID: iface.guid,
			InterfaceDescription: iface.interfaceDescription,
			InterfaceState: iface.isState,
		}
		var available uintptr
		ret, _, _ := procWLANGetAvailableNetworkList.Call(uintptr(wh.handle), uintptr(unsafe.Pointer(&iface.guid)), 0, 0, uintptr(unsafe.Pointer(&available)))
		if ret != 0 {
			return nil, fmt.Errorf("WlanGetAvailableNetworkList failed with error code %d", ret)
		}
		defer procWLANFreeMemory.Call(available)
		avail := (*WLAN_AVAILABLE_NETWORK_LIST)(unsafe.Pointer(available))
		nets := unsafe.Slice((*WLAN_AVAILABLE_NETWORK)(unsafe.Pointer(&avail.networks)), int(avail.dwNumberOfItems))
		ni.Networks = make([]WLANNetwork, 0, len(nets))

		for _, net := range nets {
			var wlnet WLANNetwork
			wlnet.NetworkID = windows.UTF16ToString(net.networkID[:])
			wlnet.SSID = string(net.dot11Ssid.ssid[:net.dot11Ssid.len])
			wlnet.BssType = net.dot11BssType

			wlnet.Connectable = net.bNetworkConnectable
			wlnet.Connected = net.flags & 0x00000001 != 0
			wlnet.HasProfile = net.flags & 0x00000002 != 0
			if !wlnet.Connectable {
				wlnet.NotConnectableReason = net.wlanNotConnectableReason
			}

			if net.signalQuality == 0 {
				wlnet.SignalStrength = -100
			} else if net.signalQuality == 100 {
				wlnet.SignalStrength = -50
			} else {	
				wlnet.SignalStrength = -100 + (net.signalQuality / 2)
			}
			ni.Networks = append(ni.Networks, wlnet)
		}
		ifaces = append(ifaces, ni)
	}
	return ifaces, nil
}