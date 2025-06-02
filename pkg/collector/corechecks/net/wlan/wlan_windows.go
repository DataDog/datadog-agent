// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
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

	iphlpapi        = windows.NewLazyDLL("iphlpapi.dll")
	getAdaptersInfo = iphlpapi.NewProc("GetAdaptersInfo")

	ole32           = windows.NewLazyDLL("ole32.dll")
	clsidFromString = ole32.NewProc("CLSIDFromString")
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

// Constants
const (
	MAX_ADAPTER_NAME_LENGTH        = 256
	MAX_ADAPTER_DESCRIPTION_LENGTH = 128
	MAX_ADAPTER_ADDRESS_LENGTH     = 8
)

// MIB_IF_TYPE constants (WiFi is usually IF_TYPE_IEEE80211 - 71)
const (
	IF_TYPE_OTHER     = 1
	IF_TYPE_ETHERNET  = 6
	IF_TYPE_IEEE80211 = 71
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

// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/ns-wlanapi-wlan_bss_entry
// not used now but may be used in future
// type WLAN_BSS_ENTRY struct {
// 	dot11Ssid               DOT11_SSID
// 	uPhyId                  uint32
// 	dot11Bssid              [6]byte
// 	dot11BssType            uint32
// 	dot11BssPhyType         uint32
// 	lrssi                   int32
// 	linkQuality             uint32
// 	bInRegDomain            byte
// 	usBeaconPeriod          uint16
// 	ullTimestamp            uint64
// 	ullHostTimestamp        uint64
// 	usCapabilityInformation uint16
// 	ulChCenterFrequency     uint32
// 	wlanRateSet             [256]byte
// 	ulIeOffset              uint32
// 	ulIeSize                uint32
// }

// type WLAN_BSS_LIST struct {
// 	dwTotalSize     uint32
// 	dwNumberOfItems uint32
// 	wlanBssEntries  [1]WLAN_BSS_ENTRY
// }

// type BssInfo struct {
// 	lrssi               int32
// 	linkQuality         uint32
// 	ulChCenterFrequency uint32
// }

// IP_ADDR_STRING
// https://learn.microsoft.com/en-us/windows/win32/api/iptypes/ns-iptypes-ip_addr_string
type IP_ADDR_STRING struct {
	Next      *IP_ADDR_STRING
	IpAddress [16]byte
	IpMask    [16]byte
	Context   uint32
}

// IP_ADAPTER_INFO
// https://learn.microsoft.com/en-us/windows/win32/api/iptypes/ns-iptypes-ip_adapter_info
type IP_ADAPTER_INFO struct {
	Next                *IP_ADAPTER_INFO
	ComboIndex          uint32
	AdapterName         [MAX_ADAPTER_NAME_LENGTH + 4]byte
	Description         [MAX_ADAPTER_DESCRIPTION_LENGTH + 4]byte
	AddressLength       uint32
	Address             [MAX_ADAPTER_ADDRESS_LENGTH]byte
	Index               uint32
	Type                uint32
	DhcpEnabled         uint32
	CurrentIpAddress    *IP_ADDR_STRING
	IpAddressList       IP_ADDR_STRING
	GatewayList         IP_ADDR_STRING
	DhcpServer          IP_ADDR_STRING
	HaveWins            uint32
	PrimaryWinsServer   IP_ADDR_STRING
	SecondaryWinsServer IP_ADDR_STRING
	LeaseObtained       uint32
	LeaseExpires        uint32
}

type adapterInfo struct {
	macAddress string
	ipAddress  string
}

type wlanInfo struct {
	ssid       string
	bssid      string
	phy        string
	auth       string
	rssi       int32
	rxRate     float64
	txRate     float64
	channel    uint32
	signal     uint32
	adapterMac string
	adapterIp  string
}

// ------------------------------------------------------
//
// # U t i l i t y   f u n c t i o n s
//
// ------------------------------------------------------
// determines the WiFi band based on frequency
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

func strToGUID(guidStr string) (*windows.GUID, error) {
	guidWStr, err := windows.UTF16PtrFromString(guidStr)
	if err != nil {
		return nil, err
	}

	var guid windows.GUID
	ret, _, _ := clsidFromString.Call(uintptr(unsafe.Pointer(guidWStr)), uintptr(unsafe.Pointer(&guid)))
	if ret != 0 {
		return nil, fmt.Errorf("clsidFromString failed")
	}

	return &guid, nil
}

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

// Get BSS information
// https://learn.microsoft.com/en-us/windows/win32/api/wlanapi/nf-wlanapi-wlangetnetworkbsslist
// Collects a list of basic service set (BSS) entries from a wireless LAN network,
// but specific for a given SSID and BSSID and returns the first entry found, which includes
// the link quality, RSSI, and channel center frequency.
//
// Currently not used but may be used in future
// func getBssInfo(wlanClient uintptr, itfGuid windows.GUID, ssid DOT11_SSID, dot11Bssid []byte) *BssInfo {
// 	var bssList *WLAN_BSS_LIST
// 	ret, _, _ := wlanGetNetworkBssList.Call(
// 		wlanClient, uintptr(unsafe.Pointer(&itfGuid)), 0, uintptr(unsafe.Pointer(&ssid)), 0, 0, uintptr(unsafe.Pointer(&bssList)))
// 	if ret != 0 {
// 		return nil
// 	}
// 	defer wlanFreeMemory.Call(uintptr(unsafe.Pointer(bssList))) //nolint:errcheck
//
// 	if bssList.dwNumberOfItems > 0 {
// 		base := unsafe.Pointer(&bssList.wlanBssEntries[0])
// 		size := unsafe.Sizeof(WLAN_BSS_ENTRY{})
// 		var i uint32 = 0
// 		for i = 0; i < bssList.dwNumberOfItems; i++ {
// 			bssEntry := (*WLAN_BSS_ENTRY)(unsafe.Pointer(uintptr(base) + uintptr(i)*size))
//			bssMac, _ := formatMacAddress(bssEntry.dot11Bssid[:])
//			itfMac, _ := formatMacAddress(dot11Bssid)
// 			if bssMac == dot11Bssid && len(itfMac) > 0 {
// 				return &BssInfo{
// 					lrssi:               bssEntry.lrssi,
// 					linkQuality:         bssEntry.linkQuality,
// 					ulChCenterFrequency: bssEntry.ulChCenterFrequency,
// 				}
// 			}
// 		}
// 	}
//
// 	return nil
// }

// ----------------
//
// # Adapter wrapper
//
// https://learn.microsoft.com/en-us/windows/win32/api/iphlpapi/nf-iphlpapi-getadaptersinfo
// Find adapters based on the corresponding Wlan interface GUID/name
// and return the MAC address and IP address of the adapter.
func getAdapter(adapterGuid windows.GUID) (*adapterInfo, error) {
	var size uint32
	ret, _, _ := getAdaptersInfo.Call(0, uintptr(unsafe.Pointer(&size)))
	if ret != uintptr(windows.ERROR_BUFFER_OVERFLOW) {
		return nil, fmt.Errorf("GetAdaptersInfo call failed. Error: %d", ret)
	}

	buffer := make([]byte, size)
	ret, _, _ = getAdaptersInfo.Call(uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if ret != 0 {
		return nil, fmt.Errorf("GetAdaptersInfo call failed. Error: %d", ret)
	}

	adapter := (*IP_ADAPTER_INFO)(unsafe.Pointer(&buffer[0]))
	for adapter != nil {
		adapterName := strings.TrimRight(string(adapter.AdapterName[:]), "\x00")
		curAdapterGuid, err := strToGUID(adapterName)
		if err != nil {
			continue // try next adapter
		}

		if *curAdapterGuid == adapterGuid {
			var macAddress string
			if adapter.AddressLength > 0 && adapter.AddressLength < MAX_ADAPTER_ADDRESS_LENGTH {
				macAddress, _ = formatMacAddress(adapter.Address[:adapter.AddressLength])
			}

			var ipAddress string
			if adapter.IpAddressList.IpAddress[0] != 0 {
				ipAddress = strings.TrimRight(string(adapter.IpAddressList.IpAddress[:]), "\x00")
			}

			return &adapterInfo{
				macAddress: macAddress,
				ipAddress:  ipAddress,
			}, nil
		}

		adapter = adapter.Next
	}

	return nil, nil
}

// ----------------
//
// WiFi Info Aggregaror
func getWlanInfo(wlanClient uintptr, itfGuid windows.GUID) (*wlanInfo, error) {
	// Get connection attributes (if failed no point to continue)
	connAttribs, err := getWlanConnAttribs(wlanClient, itfGuid)
	if err != nil {
		return nil, fmt.Errorf("failed to get WLAN connection attributes: %v", err)
	}
	defer wlanFreeMemory.Call(uintptr(unsafe.Pointer(connAttribs))) //nolint:errcheck

	var wi wlanInfo

	// Get adapter info (if failed will continue without its details)
	adapterInfo, err := getAdapter(itfGuid)
	if err == nil {
		wi.adapterIp = adapterInfo.ipAddress
		wi.adapterMac = adapterInfo.macAddress
	}

	// Extract connection attributes
	wi.ssid = formatSSID(connAttribs.wlanAssociationAttributes.dot11Ssid)
	wi.bssid, _ = formatMacAddress(connAttribs.wlanAssociationAttributes.dot11Bssid[:])
	wi.phy = formatPhy(connAttribs.wlanAssociationAttributes.dot11PhyType)
	wi.auth = formatAuthAlgo(connAttribs.wlanSecurityAttributes.dot11AuthAlgorithm)
	wi.signal = connAttribs.wlanAssociationAttributes.wlanSignalQuality

	//Convert kbps to Mbps
	wi.rxRate = float64(connAttribs.wlanAssociationAttributes.ulRxRate) / 1000.0
	wi.txRate = float64(connAttribs.wlanAssociationAttributes.ulTxRate) / 1000.0

	// Get channel and RSSI
	channel, err := getChannel(wlanClient, itfGuid)
	if err == nil {
		wi.channel = channel
	}
	rssi, err := getRssi(wlanClient, itfGuid)
	if err == nil {
		wi.rssi = rssi
	}

	// Currently will not be used but if we will need band information call getBssInfo
	// it will also can return duplicative details about lrssi and linkQuality,
	//
	// bssInfo := getBssInfo(wlanClient, itfGuid, connAttribs.wlanAssociationAttributes.dot11Ssid, connAttribs.wlanAssociationAttributes.dot11Bssid[:])

	return &wi, nil
}

func getFirstConnectedWlanInfo() (*wlanInfo, error) {
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

	// Check if any WiFi interfaces found
	if itfs.NumberOfItems == 0 {
		return nil, nil
	}

	// Iterate over WiFi interfaces until one is connected
	for i := 0; i < int(itfs.NumberOfItems); i++ {
		itf := &itfs.InterfaceInfo[i]

		// Check if interface is connected
		if itf.IsState != uint32(wlanInterfaceStateConnected) {
			continue
		}

		wi, err := getWlanInfo(wlanClient, itf.InterfaceGUID)
		if err != nil {
			return nil, fmt.Errorf("failed to get WLAN info: %v", err)
		}
		return wi, nil
	}

	return nil, nil
}

func GetWiFiInfo() (wifiInfo, error) {
	wi, err := getFirstConnectedWlanInfo()
	if err != nil {
		return wifiInfo{}, err
	}

	// If no connected WiFi interface found, return empty wifiInfo
	if wi == nil {
		return wifiInfo{phyMode: "None"}, nil
	}

	// For majority of cases for connected WiFi interface, return its details
	return wifiInfo{
		rssi:             int(wi.rssi),
		ssid:             wi.ssid,
		bssid:            wi.bssid,
		channel:          int(wi.channel),
		transmitRate:     float64(wi.txRate),
		receiveRate:      float64(wi.rxRate),
		receiveRateValid: true,
		macAddress:       wi.adapterMac,
		phyMode:          wi.phy,
	}, nil
}
