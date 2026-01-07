// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// WLAN API structures and constants
const (
	WLAN_API_VERSION = 2
)

var (
	wlanAPI            = windows.NewLazyDLL("wlanapi.dll")
	wlanOpenHandle     = wlanAPI.NewProc("WlanOpenHandle")
	wlanCloseHandle    = wlanAPI.NewProc("WlanCloseHandle")
	wlanEnumInterfaces = wlanAPI.NewProc("WlanEnumInterfaces")
	wlanQueryInterface = wlanAPI.NewProc("WlanQueryInterface")
	// wlanGetNetworkBssList = wlanAPI.NewProc("WlanGetNetworkBssList")
	wlanFreeMemory = wlanAPI.NewProc("WlanFreeMemory")

	iphlpapi                   = windows.NewLazyDLL("iphlpapi.dll")
	getIfEntry2                = iphlpapi.NewProc("GetIfEntry2")
	convertInterfaceGuidToLuid = iphlpapi.NewProc("ConvertInterfaceGuidToLuid")

	// getWiFiInfo is a package-level function variable for testability
	// Tests can reassign this to mock WiFi data retrieval
	getWiFiInfo func() (wifiInfo, error)
)

// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ne-wlanapi-wlan_interface_state-r1
type WLAN_INTERFACE_STATE uint32

const (
	wlanInterfaceStateNotReady           WLAN_INTERFACE_STATE = 0 // Not ready
	wlanInterfaceStateConnected          WLAN_INTERFACE_STATE = 1 // Connected
	wlanInterfaceStateAdHocNetworkFormed WLAN_INTERFACE_STATE = 2 // First node in a ad hoc network
	wlanInterfaceStateDisconnecting      WLAN_INTERFACE_STATE = 3 // Disconnecting
	wlanInterfaceStateDisconnected       WLAN_INTERFACE_STATE = 4 // Not connected
	wlanInterfaceStateAssociating        WLAN_INTERFACE_STATE = 5 // Attempting to associate with a network
	wlanInterfaceStateDiscovering        WLAN_INTERFACE_STATE = 6 // Auto configuration is discovering settings for the network
	wlanInterfaceStateAuthenticating     WLAN_INTERFACE_STATE = 7 // In process of authenticating
)

// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ne-wlanapi-wlan_intf_opcode
type WLAN_INTF_OPCODE uint32

const (
	wlanIntfOpcodeAutoconfStart                          WLAN_INTF_OPCODE = 0x00000000
	wlanIntfOpcodeAutoconfEnabled                        WLAN_INTF_OPCODE = 0x00000001
	wlanIntfOpcodeBackgroundScanEnabled                  WLAN_INTF_OPCODE = 0x00000002
	wlanIntfOpcodeMediaStreamingMode                     WLAN_INTF_OPCODE = 0x00000003
	wlanIntfOpcodeRadioState                             WLAN_INTF_OPCODE = 0x00000004
	wlanIntfOpcodeBssType                                WLAN_INTF_OPCODE = 0x00000005
	wlanIntfOpcodeInterfaceState                         WLAN_INTF_OPCODE = 0x00000006
	wlanIntfOpcodeCurrentConnection                      WLAN_INTF_OPCODE = 0x00000007
	wlanIntfOpcodeChannelNumber                          WLAN_INTF_OPCODE = 0x00000008
	wlanIntfOpcodeSupportedInfrastructureAuthCipherPairs WLAN_INTF_OPCODE = 0x00000009
	wlanIntfOpcodeSupportedAdhocAuthCipherPairs          WLAN_INTF_OPCODE = 0x0000000A
	wlanIntfOpcodeSupportedCountryOrRegionStringList     WLAN_INTF_OPCODE = 0x0000000B
	wlanIntfOpcodeCurrentOperationMode                   WLAN_INTF_OPCODE = 0x0000000C
	wlanIntfOpcodeSupportedSafeMode                      WLAN_INTF_OPCODE = 0x0000000D
	wlanIntfOpcodeCertifiedSafeMode                      WLAN_INTF_OPCODE = 0x0000000E
	wlanIntfOpcodeHostedNetworkCapable                   WLAN_INTF_OPCODE = 0x0000000F
	wlanIntfOpcodeManagementFrameProtectionCapable       WLAN_INTF_OPCODE = 0x00000010
	wlanIntfOpcodeSecondaryStaInterfaces                 WLAN_INTF_OPCODE = 0x00000011
	wlanIntfOpcodeSecondaryStaSynchronizedConnections    WLAN_INTF_OPCODE = 0x00000012
	wlanIntfOpcodeAutoconfEnd                            WLAN_INTF_OPCODE = 0x0FFFFFFF
	wlanIntfOpcodeMsmStart                               WLAN_INTF_OPCODE = 0x10000100
	wlanIntfOpcodeStatistics                             WLAN_INTF_OPCODE = 0x10000101
	wlanIntfOpcodeRssi                                   WLAN_INTF_OPCODE = 0x10000102
	wlanIntfOpcodeMsmEnd                                 WLAN_INTF_OPCODE = 0x1FFFFFFF
	wlanIntfOpcodeSecurityStart                          WLAN_INTF_OPCODE = 0x20010000
	wlanIntfOpcodeSecurityEnd                            WLAN_INTF_OPCODE = 0x2FFFFFFF
	wlanIntfOpcodeIhvStart                               WLAN_INTF_OPCODE = 0x30000000
	wlanIntfOpcodeIhvEnd                                 WLAN_INTF_OPCODE = 0x3FFFFFFF
)

// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ne-wlanapi-wlan_connection_mode
type WLAN_CONNECTION_MODE uint32

// const (
// 	wlanConnectionModeProfile           WLAN_CONNECTION_MODE = 0 // A profile is used to make the connection
// 	wlanConnectionModeTemporaryProfile  WLAN_CONNECTION_MODE = 1 // A temporary profile is used to make the connection
// 	wlanConnectionModeDiscoverySecure   WLAN_CONNECTION_MODE = 2 // Secure discovery is used to make the connection
// 	wlanConnectionModeDiscoveryUnsecure WLAN_CONNECTION_MODE = 3 // Unsecure discovery is used to make the connection
// 	wlanConnectionModeAuto              WLAN_CONNECTION_MODE = 4 // Connection initiated by wireless service automatically using a persistent profile
// 	wlanConnectionModeInvalid           WLAN_CONNECTION_MODE = 5 // Invalid connection mode
// )

// https://learn.microsoft.com/en-us/windows/win32/nativewifi/dot11-phy-type
type DOT11_PHY_TYPE uint32

const (
	dot11PhyTypeUnknown    DOT11_PHY_TYPE = 0
	dot11PhyTypeAny        DOT11_PHY_TYPE = dot11PhyTypeUnknown
	dot11PhyTypeFhss       DOT11_PHY_TYPE = 1
	dot11PhyTypeDsss       DOT11_PHY_TYPE = 2
	dot11PhyTypeIrBaseband DOT11_PHY_TYPE = 3
	dot11PhyTypeOfdm       DOT11_PHY_TYPE = 4  // 11a
	dot11PhyTypeHrDsss     DOT11_PHY_TYPE = 5  // 11b
	dot11PhyTypeErp        DOT11_PHY_TYPE = 6  // 11g
	dot11PhyTypeHt         DOT11_PHY_TYPE = 7  // 11n
	dot11PhyTypeVht        DOT11_PHY_TYPE = 8  // 11ac
	dot11PhyTypeDmg        DOT11_PHY_TYPE = 9  // 11ad
	dot11PhyTypeHe         DOT11_PHY_TYPE = 10 // 11ax
	dot11PhyTypeEht        DOT11_PHY_TYPE = 11 // 11be
	dot11PhyTypeIHVStart   DOT11_PHY_TYPE = 0x80000000
	dot11PhyTypeIHVEnd     DOT11_PHY_TYPE = 0xffffffff
)

// https://learn.microsoft.com/en-us/windows/win32/nativewifi/dot11-auth-algorithm
type DOT11_AUTH_ALGORITHM uint32

const (
	DOT11_AUTH_ALGO_80211_OPEN       DOT11_AUTH_ALGORITHM = 1
	DOT11_AUTH_ALGO_80211_SHARED_KEY DOT11_AUTH_ALGORITHM = 2
	DOT11_AUTH_ALGO_WPA              DOT11_AUTH_ALGORITHM = 3
	DOT11_AUTH_ALGO_WPA_PSK          DOT11_AUTH_ALGORITHM = 4
	DOT11_AUTH_ALGO_WPA_NONE         DOT11_AUTH_ALGORITHM = 5 // used in NatSTA only
	DOT11_AUTH_ALGO_RSNA             DOT11_AUTH_ALGORITHM = 6
	DOT11_AUTH_ALGO_RSNA_PSK         DOT11_AUTH_ALGORITHM = 7
	DOT11_AUTH_ALGO_WPA3             DOT11_AUTH_ALGORITHM = 8 // means WPA3 Enterprise 192 bits
	DOT11_AUTH_ALGO_WPA3_ENT_192     DOT11_AUTH_ALGORITHM = DOT11_AUTH_ALGO_WPA3
	DOT11_AUTH_ALGO_WPA3_SAE         DOT11_AUTH_ALGORITHM = 9
	DOT11_AUTH_ALGO_OWE              DOT11_AUTH_ALGORITHM = 10
	DOT11_AUTH_ALGO_WPA3_ENT         DOT11_AUTH_ALGORITHM = 11
	DOT11_AUTH_ALGO_IHV_START        DOT11_AUTH_ALGORITHM = 0x80000000
	DOT11_AUTH_ALGO_IHV_END          DOT11_AUTH_ALGORITHM = 0xffffffff
)

// https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/wlantypes/ne-wlantypes-_dot11_cipher_algorithm
type DOT11_CIPHER_ALGORITHM uint32

const (
	DOT11_CIPHER_ALGO_NONE          DOT11_CIPHER_ALGORITHM = 0x00
	DOT11_CIPHER_ALGO_WEP40         DOT11_CIPHER_ALGORITHM = 0x01
	DOT11_CIPHER_ALGO_TKIP          DOT11_CIPHER_ALGORITHM = 0x02
	DOT11_CIPHER_ALGO_CCMP          DOT11_CIPHER_ALGORITHM = 0x04
	DOT11_CIPHER_ALGO_WEP104        DOT11_CIPHER_ALGORITHM = 0x05
	DOT11_CIPHER_ALGO_BIP           DOT11_CIPHER_ALGORITHM = 0x06
	DOT11_CIPHER_ALGO_GCMP          DOT11_CIPHER_ALGORITHM = 0x08
	DOT11_CIPHER_ALGO_GCMP_256      DOT11_CIPHER_ALGORITHM = 0x09
	DOT11_CIPHER_ALGO_CCMP_256      DOT11_CIPHER_ALGORITHM = 0x0a
	DOT11_CIPHER_ALGO_BIP_GMAC_128  DOT11_CIPHER_ALGORITHM = 0x0b
	DOT11_CIPHER_ALGO_BIP_GMAC_256  DOT11_CIPHER_ALGORITHM = 0x0c
	DOT11_CIPHER_ALGO_BIP_CMAC_256  DOT11_CIPHER_ALGORITHM = 0x0d
	DOT11_CIPHER_ALGO_WPA_USE_GROUP DOT11_CIPHER_ALGORITHM = 0x100
	DOT11_CIPHER_ALGO_RSN_USE_GROUP DOT11_CIPHER_ALGORITHM = 0x100
	DOT11_CIPHER_ALGO_WEP           DOT11_CIPHER_ALGORITHM = 0x101
	DOT11_CIPHER_ALGO_IHV_START     DOT11_CIPHER_ALGORITHM = 0x80000000
	DOT11_CIPHER_ALGO_IHV_END       DOT11_CIPHER_ALGORITHM = 0xffffffff
)

// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ns-wlanapi-wlan_interface_info
type WLAN_INTERFACE_INFO struct {
	InterfaceGUID           windows.GUID
	strInterfaceDescription [256]uint16
	IsState                 uint32 // WLAN_INTERFACE_STATE
}

// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ns-wlanapi-wlan_interface_info_list
type WLAN_INTERFACE_INFO_LIST struct {
	NumberOfItems uint32
	Index         uint32
	InterfaceInfo [1]WLAN_INTERFACE_INFO
}

type DOT11_SSID struct {
	USSIDLength uint32
	UcSSID      [32]byte
}

// structure WLAN_ASSOCIATION_ATTRIBUTES defines attributes of a wireless
// association. The unit for Rx/Tx rate is Kbits/second.
// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ns-wlanapi-wlan_association_attributes
type WLAN_ASSOCIATION_ATTRIBUTES struct {
	dot11Ssid         DOT11_SSID
	dot11BssType      uint32
	dot11Bssid        [6]byte
	dot11PhyType      DOT11_PHY_TYPE // DOT11_PHY_TYPE
	uDot11PhyIndex    uint32
	wlanSignalQuality uint32
	ulRxRate          uint32
	ulTxRate          uint32
}

type WLAN_SECURITY_ATTRIBUTES struct {
	securityEnabled      int32
	oneXEnabled          int32
	dot11AuthAlgorithm   DOT11_AUTH_ALGORITHM
	dot11CipherAlgorithm DOT11_CIPHER_ALGORITHM
}

// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ns-wlanapi-wlan_connection_attributes
type WLAN_CONNECTION_ATTRIBUTES struct {
	isState                   uint32 // WLAN_INTERFACE_STATE
	wlanConnectionMode        uint32 // WLAN_CONNECTION_MODE
	profileName               [256]uint16
	wlanAssociationAttributes WLAN_ASSOCIATION_ATTRIBUTES
	wlanSecurityAttributes    WLAN_SECURITY_ATTRIBUTES
}

// ------------------------------------------------------
//
// # U t i l i t y   f u n c t i o n s
//
// ------------------------------------------------------
// determines the Wi-Fi band based on frequency
//  Currently not used but may be used in future
//
// func formatBand(frequency uint32) string {
// 	if frequency >= 2412000 && frequency <= 2484000 {
// 		return "2.4 GHz"
// 	} else if frequency >= 5180000 && frequency <= 5825000 {
// 		return "5 GHz"
// 	} else if frequency >= 5925000 {
// 		return "6 GHz"
// 	}
// 	return "Unknown"
// }

func formatSSID(dot11Ssid DOT11_SSID) string {
	ssidLen := dot11Ssid.USSIDLength
	if ssidLen == 0 {
		return ""
	}

	return string(dot11Ssid.UcSSID[:ssidLen])
}

func formatMacAddress(mac []byte) (string, error) {
	// MAC address should be 6 bytes
	if len(mac) != 6 {
		return "", fmt.Errorf("invalid MAC address length (provided %d, expected 6)", len(mac))
	}
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]), nil
}

func formatPhy(phy DOT11_PHY_TYPE) string {
	// PHY type
	phyStr := ""
	switch phy {
	case dot11PhyTypeAny:
		phyStr = "Any"
	case dot11PhyTypeFhss:
		phyStr = "FHSS"
	case dot11PhyTypeDsss:
		phyStr = "DSSS"
	case dot11PhyTypeIrBaseband:
		phyStr = "IR Baseband"
	case dot11PhyTypeOfdm:
		phyStr = "802.11a"
	case dot11PhyTypeHrDsss:
		phyStr = "802.11b"
	case dot11PhyTypeErp:
		phyStr = "802.11g"
	case dot11PhyTypeHt:
		phyStr = "802.11n"
	case dot11PhyTypeVht:
		phyStr = "802.11ac"
	case dot11PhyTypeDmg:
		phyStr = "802.11ad"
	case dot11PhyTypeHe:
		phyStr = "802.11ax"
	case dot11PhyTypeEht:
		phyStr = "802.11be"
	}

	return phyStr
}

// Not needed for now but may be used in future
/*
func formatAuthAlgo(authAlgo DOT11_AUTH_ALGORITHM) string {
	authAlgoStr := ""
	switch authAlgo {
	case DOT11_AUTH_ALGO_80211_OPEN:
		authAlgoStr = "802.11 Open"
	case DOT11_AUTH_ALGO_80211_SHARED_KEY:
		authAlgoStr = "802.11 Shared Key"
	case DOT11_AUTH_ALGO_WPA:
		authAlgoStr = "WPA"
	case DOT11_AUTH_ALGO_WPA_PSK:
		authAlgoStr = "WPA-PSK"
	case DOT11_AUTH_ALGO_WPA_NONE:
		authAlgoStr = "WPA None"
	case DOT11_AUTH_ALGO_RSNA:
		authAlgoStr = "RSNA"
	case DOT11_AUTH_ALGO_RSNA_PSK:
		authAlgoStr = "RSNA-PSK (WPA2-Personal)"
	case DOT11_AUTH_ALGO_WPA3:
		authAlgoStr = "WPA3"
	case DOT11_AUTH_ALGO_WPA3_SAE:
		authAlgoStr = "WPA3-SAE"
	case DOT11_AUTH_ALGO_OWE:
		authAlgoStr = "OWE"
	case DOT11_AUTH_ALGO_WPA3_ENT:
		authAlgoStr = "WPA3-Enterprise"
	}

	return authAlgoStr
}
*/

// ----------------
//
// wlanQueryInterface.Call wrappers
//
// ----------------
// Get WLAN connection attributes
func getWlanConnAttribs(wlanClient uintptr, itfGuid windows.GUID) (*WLAN_CONNECTION_ATTRIBUTES, error) {
	// Get current connection attributes
	// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/nf-wlanapi-wlanqueryinterface
	var dataSize uint32
	var conn *WLAN_CONNECTION_ATTRIBUTES
	ret, _, _ := wlanQueryInterface.Call(wlanClient, uintptr(unsafe.Pointer(&itfGuid)),
		uintptr(wlanIntfOpcodeCurrentConnection), 0, uintptr(unsafe.Pointer(&dataSize)), uintptr(unsafe.Pointer(&conn)), 0)
	if ret != 0 {
		return nil, fmt.Errorf("wlanQueryInterface failed: %d", ret)
	}
	return conn, nil
}

func getChannel(wlanClient uintptr, itfGuid windows.GUID) (uint32, error) {
	var dataSize uint32
	var data *uint32

	ret, _, _ := wlanQueryInterface.Call(
		wlanClient, uintptr(unsafe.Pointer(&itfGuid)), uintptr(wlanIntfOpcodeChannelNumber), 0, uintptr(unsafe.Pointer(&dataSize)), uintptr(unsafe.Pointer(&data)), 0)
	if ret != 0 {
		return 0, fmt.Errorf("wlanQueryInterface failed: %d", ret)
	}
	defer wlanFreeMemory.Call(uintptr(unsafe.Pointer(data))) //nolint:errcheck

	channel := *data
	return channel, nil
}

func getRssi(wlanClient uintptr, itfGuid windows.GUID) (int32, error) {
	var dataSize uint32
	var data *int32

	ret, _, _ := wlanQueryInterface.Call(
		wlanClient, uintptr(unsafe.Pointer(&itfGuid)), uintptr(wlanIntfOpcodeRssi), 0, uintptr(unsafe.Pointer(&dataSize)), uintptr(unsafe.Pointer(&data)), 0)
	if ret != 0 {
		return 0, fmt.Errorf("wlanQueryInterface failed: %d", ret)
	}
	defer wlanFreeMemory.Call(uintptr(unsafe.Pointer(data))) //nolint:errcheck

	rssi := *data
	return rssi, nil
}

// ----------------
//
// # Other wlanxxx.Call wrappers
//
// ----------------
func getWlanHandle() (uintptr, error) {
	var wlanClient uintptr
	var negVer uint32
	ret, _, _ := wlanOpenHandle.Call(uintptr(WLAN_API_VERSION), 0, uintptr(unsafe.Pointer(&negVer)), uintptr(unsafe.Pointer(&wlanClient)))
	if ret != 0 {
		return 0, fmt.Errorf("wlanOpenHandle failed: %d", ret)
	}
	return wlanClient, nil
}

func getWlanInterfacesList(wlanClient uintptr) (*WLAN_INTERFACE_INFO_LIST, error) {
	// Get WLAN interfaces
	// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/nf-wlanapi-wlanenuminterfaces
	var itfs *WLAN_INTERFACE_INFO_LIST
	ret, _, _ := wlanEnumInterfaces.Call(wlanClient, 0, uintptr(unsafe.Pointer(&itfs)))
	if ret != 0 {
		return nil, fmt.Errorf("wlanEnumInterfaces failed: %d", ret)
	}
	return itfs, nil
}

// https://learn.microsoft.com/en-us/windows/win32/api/netioapi/nf-netioapi-getifentry2
// https://learn.microsoft.com/en-us/windows/win32/api/netioapi/nf-netioapi-convertinterfaceguidtoluid
// Use GetIfEntry2() for the wlan Interface GUID
func getWlanMacAddr(wlanItfGuid windows.GUID) (string, error) {
	// row will be initialized to 0 (by Go standard)
	var row windows.MibIfRow2

	// To find Interface details via GetIfEntry2() function, its InterfaceLuid or InterfaceIndex fields need to be set.
	// We will set InterfaceLuid field by converting passed Interface Guid to its Luid via ConvertInterfaceGuidToLuid.
	ret, _, _ := convertInterfaceGuidToLuid.Call(
		uintptr(unsafe.Pointer(&wlanItfGuid)),
		uintptr(unsafe.Pointer(&row.InterfaceLuid)))
	if ret != 0 {
		return "", fmt.Errorf("ConvertInterfaceGuidToLuid call failed. Error: %d", ret)
	}
	ret, _, _ = getIfEntry2.Call(uintptr(unsafe.Pointer(&row)))
	if ret != 0 {
		return "", fmt.Errorf("GetIfEntry2 call failed. Error: %d", ret)
	}

	if row.Type == windows.IF_TYPE_IEEE80211 && row.PhysicalAddressLength > 0 && row.PhysicalAddressLength <= 32 {
		return formatMacAddress(row.PhysicalAddress[:row.PhysicalAddressLength])
	}

	log.Warnf("Cannot get mac address for interface %v", wlanItfGuid)
	return "", nil
}

// ----------------
//
// Wi-Fi Info Aggregaror
func getWlanInfo(wlanClient uintptr, wlanItfGuild windows.GUID) (*wifiInfo, error) {
	// Get connection attributes (if failed no point to continue)
	connAttribs, err := getWlanConnAttribs(wlanClient, wlanItfGuild)
	if err != nil {
		return nil, fmt.Errorf("failed to get WLAN connection attributes: %v", err)
	}
	defer wlanFreeMemory.Call(uintptr(unsafe.Pointer(connAttribs))) //nolint:errcheck

	var wi wifiInfo

	// Get adapter info (if failed will continue without its details)
	macAddr, err := getWlanMacAddr(wlanItfGuild)
	if err == nil && len(macAddr) > 0 {
		wi.macAddress = macAddr
	} else if err != nil {
		log.Errorf("failed to get WLAN interface mac address: %v", err)
	}

	// Extract connection attributes
	wi.ssid = formatSSID(connAttribs.wlanAssociationAttributes.dot11Ssid)
	wi.bssid, _ = formatMacAddress(connAttribs.wlanAssociationAttributes.dot11Bssid[:])
	wi.phyMode = formatPhy(connAttribs.wlanAssociationAttributes.dot11PhyType)
	// wi.auth = formatAuthAlgo(connAttribs.wlanSecurityAttributes.dot11AuthAlgorithm)
	// wi.signal = connAttribs.wlanAssociationAttributes.wlanSignalQuality

	//Convert kbps to Mbps
	wi.receiveRate = float64(connAttribs.wlanAssociationAttributes.ulRxRate) / 1000.0
	wi.transmitRate = float64(connAttribs.wlanAssociationAttributes.ulTxRate) / 1000.0

	// Get channel and RSSI
	channel, err := getChannel(wlanClient, wlanItfGuild)
	if err == nil {
		wi.channel = int(channel)
	}
	rssi, err := getRssi(wlanClient, wlanItfGuild)
	if err == nil {
		wi.rssi = int(rssi)
	}

	return &wi, nil
}

func getFirstConnectedWlanInfo() (*wifiInfo, error) {
	// Get WLAN client handle
	wlanClient, err := getWlanHandle()
	if err != nil {
		return nil, fmt.Errorf("failed to get WLAN client handle: %v", err)
	}
	defer wlanCloseHandle.Call(wlanClient, 0) //nolint:errcheck

	// Get WLAN interfaces
	itfs, err := getWlanInterfacesList(wlanClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get WLAN interfaces: %v", err)
	}
	defer wlanFreeMemory.Call(uintptr(unsafe.Pointer(itfs))) //nolint:errcheck

	// Check if any Wi-Fi interfaces found
	if itfs.NumberOfItems == 0 {
		log.Tracef("No Wi-Fi interfaces are found")
		return nil, nil
	}

	// Iterate over Wi-Fi interfaces until one is connected
	for i := 0; i < int(itfs.NumberOfItems); i++ {
		itf := &itfs.InterfaceInfo[i]

		// Check if interface is connected
		if itf.IsState != uint32(wlanInterfaceStateConnected) {
			log.Tracef("WLAN interface %d (GUID=%v) not connected (state=%d), skipping", i+1, itf.InterfaceGUID, itf.IsState)
			continue
		}

		wi, err := getWlanInfo(wlanClient, itf.InterfaceGUID)
		if err != nil {
			return nil, fmt.Errorf("failed to get WLAN info: %v", err)
		}
		log.Tracef("Found WLAN connected interface %d (GUID=%v)", i+1, itf.InterfaceGUID)
		return wi, nil
	}

	log.Tracef("No connected WLAN interfaces are found")
	return nil, nil
}

// GetWiFiInfo retrieves WiFi information on Windows
func (c *WLANCheck) GetWiFiInfo() (wifiInfo, error) {
	// Check for test override
	if getWiFiInfo != nil {
		return getWiFiInfo()
	}

	wi, err := getFirstConnectedWlanInfo()
	if err != nil {
		return wifiInfo{}, err
	}

	// If no connected Wi-Fi interface found, return empty wifiInfo
	if wi == nil {
		return wifiInfo{phyMode: "None"}, nil
	}

	// For majority of cases for connected Wi-Fi interface, return its details
	wi.receiveRateValid = true
	return *wi, nil
}
