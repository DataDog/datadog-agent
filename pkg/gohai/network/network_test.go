// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package network

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

func TestCollect(t *testing.T) {
	netInfo, err := CollectInfo()
	require.NoError(t, err)

	assertIPv4(t, netInfo.IPAddress)
	assertValueIPv6(t, netInfo.IPAddressV6)
	assertMac(t, netInfo.MacAddress)

	require.NotEmpty(t, netInfo.Interfaces)
	for _, iface := range netInfo.Interfaces {
		assert.NotEmpty(t, iface.Name)

		assertValueCIDR(t, iface.IPv4Network)
		assertValueCIDR(t, iface.IPv6Network)
		assertValueMac(t, iface.MacAddress)

		for _, addr := range iface.IPv4 {
			assertIPv4(t, addr)
		}
		for _, addr := range iface.IPv6 {
			assertIPv6(t, addr)
		}
	}
}

// mockNetworkInterface implements networkInterface for testing
type mockNetworkInterface struct {
	name         string
	flags        net.Flags
	hardwareAddr net.HardwareAddr
	addrs        []net.Addr
	addrsErr     error
}

func (m *mockNetworkInterface) GetName() string                   { return m.name }
func (m *mockNetworkInterface) GetFlags() net.Flags               { return m.flags }
func (m *mockNetworkInterface) GetHardwareAddr() net.HardwareAddr { return m.hardwareAddr }
func (m *mockNetworkInterface) Addrs() ([]net.Addr, error)        { return m.addrs, m.addrsErr }

// setMockInterfaces sets up mock interfaces for testing and registers cleanup with t.Cleanup
func setMockInterfaces(t *testing.T, ifaces []networkInterface, err error) {
	t.Helper()
	original := getInterfaces
	getInterfaces = func() ([]networkInterface, error) {
		return ifaces, err
	}
	t.Cleanup(func() {
		getInterfaces = original
	})
}

// createMockInterface creates a mockNetworkInterface with the given parameters
func createMockInterface(name string, flags net.Flags, hwAddr string, addrs []net.Addr) *mockNetworkInterface {
	var hw net.HardwareAddr
	if hwAddr != "" {
		hw, _ = net.ParseMAC(hwAddr)
	}
	return &mockNetworkInterface{
		name:         name,
		flags:        flags,
		hardwareAddr: hw,
		addrs:        addrs,
	}
}

// createIPNetAddr creates a *net.IPNet address from a CIDR string
func createIPNetAddr(cidr string) *net.IPNet {
	ip, ipnet, _ := net.ParseCIDR(cidr)
	ipnet.IP = ip
	return ipnet
}

func TestCollectInfo(t *testing.T) {
	tests := []struct {
		name           string
		interfaces     []networkInterface
		interfacesErr  error
		expectedErr    bool
		expectedIPv4   string
		expectedIPv6   string
		expectedMac    string
		expectedIfaces int
	}{
		{
			name:          "interfaces error",
			interfacesErr: errors.New("mock error"),
			expectedErr:   true,
		},
		{
			name:        "no interfaces",
			interfaces:  []networkInterface{},
			expectedErr: true,
		},
		{
			name: "only loopback interface",
			interfaces: []networkInterface{
				createMockInterface("lo", net.FlagUp|net.FlagLoopback, "", nil),
			},
			expectedErr: true,
		},
		{
			name: "only down interface",
			interfaces: []networkInterface{
				createMockInterface("eth0", 0, "00:11:22:33:44:55", nil),
			},
			expectedErr: true,
		},
		{
			name: "single interface with IPv4",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
				}),
			},
			expectedErr:    false,
			expectedIPv4:   "192.168.1.100",
			expectedMac:    "00:11:22:33:44:55",
			expectedIfaces: 1,
		},
		{
			name: "single interface with IPv4 and IPv6",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
					createIPNetAddr("fe80::1/64"),
				}),
			},
			expectedErr:    false,
			expectedIPv4:   "192.168.1.100",
			expectedIPv6:   "fe80::1",
			expectedMac:    "00:11:22:33:44:55",
			expectedIfaces: 1,
		},
		{
			name: "multiple interfaces",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
				}),
				createMockInterface("eth1", net.FlagUp, "00:11:22:33:44:66", []net.Addr{
					createIPNetAddr("10.0.0.50/8"),
				}),
			},
			expectedErr:    false,
			expectedIPv4:   "192.168.1.100", // first interface
			expectedMac:    "00:11:22:33:44:55",
			expectedIfaces: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMockInterfaces(t, tt.interfaces, tt.interfacesErr)

			info, err := CollectInfo()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, info)

			if tt.expectedIPv4 != "" {
				assert.Equal(t, tt.expectedIPv4, info.IPAddress)
			}
			if tt.expectedMac != "" {
				assert.Equal(t, tt.expectedMac, info.MacAddress)
			}
			if tt.expectedIPv6 != "" {
				ipv6, err := info.IPAddressV6.Value()
				require.NoError(t, err)
				assert.Equal(t, tt.expectedIPv6, ipv6)
			}
			assert.Len(t, info.Interfaces, tt.expectedIfaces)
		})
	}
}

func TestGetMultiNetworkInfo(t *testing.T) {
	tests := []struct {
		name           string
		interfaces     []networkInterface
		interfacesErr  error
		expectedErr    bool
		expectedIfaces int
		validate       func(t *testing.T, ifaces []Interface)
	}{
		{
			name:          "interfaces error",
			interfacesErr: errors.New("mock error"),
			expectedErr:   true,
		},
		{
			name:           "no interfaces",
			interfaces:     []networkInterface{},
			expectedIfaces: 0,
		},
		{
			name: "loopback interface is skipped",
			interfaces: []networkInterface{
				createMockInterface("lo", net.FlagUp|net.FlagLoopback, "", nil),
			},
			expectedIfaces: 0,
		},
		{
			name: "down interface is skipped",
			interfaces: []networkInterface{
				createMockInterface("eth0", 0, "00:11:22:33:44:55", nil),
			},
			expectedIfaces: 0,
		},
		{
			name: "interface up without addresses",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{}),
			},
			expectedIfaces: 1,
			validate: func(t *testing.T, ifaces []Interface) {
				require.Len(t, ifaces, 1)
				assert.Equal(t, "eth0", ifaces[0].Name)
				assert.Empty(t, ifaces[0].IPv4)
				assert.Empty(t, ifaces[0].IPv6)
				// MacAddress should be an error since there are no addresses
				_, err := ifaces[0].MacAddress.Value()
				assert.Error(t, err)
			},
		},
		{
			name: "interface with IPv4 address",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
				}),
			},
			expectedIfaces: 1,
			validate: func(t *testing.T, ifaces []Interface) {
				require.Len(t, ifaces, 1)
				assert.Equal(t, "eth0", ifaces[0].Name)
				assert.Equal(t, []string{"192.168.1.100"}, ifaces[0].IPv4)
				assert.Empty(t, ifaces[0].IPv6)

				ipv4Network, err := ifaces[0].IPv4Network.Value()
				require.NoError(t, err)
				assert.Equal(t, "192.168.1.0/24", ipv4Network)

				mac, err := ifaces[0].MacAddress.Value()
				require.NoError(t, err)
				assert.Equal(t, "00:11:22:33:44:55", mac)
			},
		},
		{
			name: "interface with IPv6 address",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("fe80::1/64"),
				}),
			},
			expectedIfaces: 1,
			validate: func(t *testing.T, ifaces []Interface) {
				require.Len(t, ifaces, 1)
				assert.Equal(t, "eth0", ifaces[0].Name)
				assert.Empty(t, ifaces[0].IPv4)
				assert.Equal(t, []string{"fe80::1"}, ifaces[0].IPv6)

				ipv6Network, err := ifaces[0].IPv6Network.Value()
				require.NoError(t, err)
				assert.Equal(t, "fe80::/64", ipv6Network)

				mac, err := ifaces[0].MacAddress.Value()
				require.NoError(t, err)
				assert.Equal(t, "00:11:22:33:44:55", mac)
			},
		},
		{
			name: "interface with both IPv4 and IPv6",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
					createIPNetAddr("fe80::1/64"),
				}),
			},
			expectedIfaces: 1,
			validate: func(t *testing.T, ifaces []Interface) {
				require.Len(t, ifaces, 1)
				assert.Equal(t, []string{"192.168.1.100"}, ifaces[0].IPv4)
				assert.Equal(t, []string{"fe80::1"}, ifaces[0].IPv6)
			},
		},
		{
			name: "interface with multiple addresses of same type",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
					createIPNetAddr("192.168.1.101/24"),
					createIPNetAddr("fe80::1/64"),
					createIPNetAddr("fe80::2/64"),
				}),
			},
			expectedIfaces: 1,
			validate: func(t *testing.T, ifaces []Interface) {
				require.Len(t, ifaces, 1)
				assert.Equal(t, []string{"192.168.1.100", "192.168.1.101"}, ifaces[0].IPv4)
				assert.Equal(t, []string{"fe80::1", "fe80::2"}, ifaces[0].IPv6)
			},
		},
		{
			name: "multiple interfaces mixed states",
			interfaces: []networkInterface{
				createMockInterface("lo", net.FlagUp|net.FlagLoopback, "", nil),
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
				}),
				createMockInterface("eth1", 0, "00:11:22:33:44:66", nil), // down
				createMockInterface("eth2", net.FlagUp, "00:11:22:33:44:77", []net.Addr{
					createIPNetAddr("10.0.0.50/8"),
				}),
			},
			expectedIfaces: 2, // only eth0 and eth2 are up and not loopback
			validate: func(t *testing.T, ifaces []Interface) {
				require.Len(t, ifaces, 2)
				assert.Equal(t, "eth0", ifaces[0].Name)
				assert.Equal(t, "eth2", ifaces[1].Name)
			},
		},
		{
			name: "interface without hardware address",
			interfaces: []networkInterface{
				createMockInterface("tun0", net.FlagUp, "", []net.Addr{
					createIPNetAddr("10.8.0.1/24"),
				}),
			},
			expectedIfaces: 1,
			validate: func(t *testing.T, ifaces []Interface) {
				require.Len(t, ifaces, 1)
				assert.Equal(t, "tun0", ifaces[0].Name)
				// MacAddress should be error for interface without hardware addr
				_, err := ifaces[0].MacAddress.Value()
				assert.Error(t, err)
			},
		},
		{
			name: "interface with Addrs() error is skipped",
			interfaces: []networkInterface{
				&mockNetworkInterface{
					name:     "eth0",
					flags:    net.FlagUp,
					addrsErr: errors.New("addrs error"),
				},
				createMockInterface("eth1", net.FlagUp, "00:11:22:33:44:66", []net.Addr{
					createIPNetAddr("10.0.0.50/8"),
				}),
			},
			expectedIfaces: 1,
			validate: func(t *testing.T, ifaces []Interface) {
				require.Len(t, ifaces, 1)
				assert.Equal(t, "eth1", ifaces[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMockInterfaces(t, tt.interfaces, tt.interfacesErr)

			ifaces, err := getMultiNetworkInfo()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, ifaces, tt.expectedIfaces)

			if tt.validate != nil {
				tt.validate(t, ifaces)
			}
		})
	}
}

func TestExternalIPAddress(t *testing.T) {
	tests := []struct {
		name          string
		interfaces    []networkInterface
		interfacesErr error
		expectedErr   bool
		expectedIP    string
	}{
		{
			name:          "interfaces error",
			interfacesErr: errors.New("mock error"),
			expectedErr:   true,
		},
		{
			name:        "no interfaces",
			interfaces:  []networkInterface{},
			expectedErr: true,
		},
		{
			name: "only loopback",
			interfaces: []networkInterface{
				createMockInterface("lo", net.FlagUp|net.FlagLoopback, "", nil),
			},
			expectedErr: true,
		},
		{
			name: "only down interface",
			interfaces: []networkInterface{
				createMockInterface("eth0", 0, "00:11:22:33:44:55", nil),
			},
			expectedErr: true,
		},
		{
			name: "interface with no addresses",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{}),
			},
			expectedErr: true,
		},
		{
			name: "interface with only IPv6",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("fe80::1/64"),
				}),
			},
			expectedErr: true,
		},
		{
			name: "interface with IPv4",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
				}),
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "returns first IPv4 from first valid interface",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
					createIPNetAddr("192.168.1.101/24"),
				}),
				createMockInterface("eth1", net.FlagUp, "00:11:22:33:44:66", []net.Addr{
					createIPNetAddr("10.0.0.50/8"),
				}),
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "skips loopback addresses",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("127.0.0.1/8"),
					createIPNetAddr("192.168.1.100/24"),
				}),
			},
			expectedIP: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMockInterfaces(t, tt.interfaces, tt.interfacesErr)

			ip, err := externalIPAddress()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedIP, ip)
		})
	}
}

func TestExternalIpv6Address(t *testing.T) {
	tests := []struct {
		name          string
		interfaces    []networkInterface
		interfacesErr error
		expectedErr   bool
		expectedIP    string
	}{
		{
			name:          "interfaces error",
			interfacesErr: errors.New("mock error"),
			expectedErr:   true,
		},
		{
			name:       "no interfaces returns empty string",
			interfaces: []networkInterface{},
			expectedIP: "",
		},
		{
			name: "only loopback returns empty string",
			interfaces: []networkInterface{
				createMockInterface("lo", net.FlagUp|net.FlagLoopback, "", nil),
			},
			expectedIP: "",
		},
		{
			name: "interface with only IPv4 returns empty string",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
				}),
			},
			expectedIP: "",
		},
		{
			name: "interface with IPv6",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("fe80::1/64"),
				}),
			},
			expectedIP: "fe80::1",
		},
		{
			name: "returns first IPv6 from first valid interface",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"), // IPv4 is skipped
					createIPNetAddr("fe80::1/64"),
					createIPNetAddr("fe80::2/64"),
				}),
			},
			expectedIP: "fe80::1",
		},
		{
			name: "skips loopback addresses",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("::1/128"),
					createIPNetAddr("fe80::1/64"),
				}),
			},
			expectedIP: "fe80::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMockInterfaces(t, tt.interfaces, tt.interfacesErr)

			ip, err := externalIpv6Address()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedIP, ip)
		})
	}
}

func TestMacAddress(t *testing.T) {
	tests := []struct {
		name          string
		interfaces    []networkInterface
		interfacesErr error
		expectedErr   bool
		expectedMac   string
	}{
		{
			name:          "interfaces error",
			interfacesErr: errors.New("mock error"),
			expectedErr:   true,
		},
		{
			name:        "no interfaces",
			interfaces:  []networkInterface{},
			expectedErr: true,
		},
		{
			name: "only loopback",
			interfaces: []networkInterface{
				createMockInterface("lo", net.FlagUp|net.FlagLoopback, "", nil),
			},
			expectedErr: true,
		},
		{
			name: "interface with no addresses",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{}),
			},
			expectedErr: true,
		},
		{
			name: "interface with only IPv6 (mac requires IPv4)",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("fe80::1/64"),
				}),
			},
			expectedErr: true,
		},
		{
			name: "interface with IPv4",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
				}),
			},
			expectedMac: "00:11:22:33:44:55",
		},
		{
			name: "returns MAC from first interface with IPv4",
			interfaces: []networkInterface{
				createMockInterface("eth0", net.FlagUp, "00:11:22:33:44:55", []net.Addr{
					createIPNetAddr("192.168.1.100/24"),
				}),
				createMockInterface("eth1", net.FlagUp, "00:11:22:33:44:66", []net.Addr{
					createIPNetAddr("10.0.0.50/8"),
				}),
			},
			expectedMac: "00:11:22:33:44:55",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMockInterfaces(t, tt.interfaces, tt.interfacesErr)

			mac, err := macAddress()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedMac, mac)
		})
	}
}

func TestAsJSON(t *testing.T) {
	tests := []struct {
		name         string
		info         *Info
		expectedKeys []string
		validate     func(t *testing.T, result map[string]interface{}, warnings []string)
	}{
		{
			name: "basic info without ipv6",
			info: &Info{
				MacAddress:  "00:11:22:33:44:55",
				IPAddress:   "192.168.1.100",
				IPAddressV6: utils.NewErrorValue[string](ErrAddressNotFound),
				Interfaces:  []Interface{},
			},
			expectedKeys: []string{"macaddress", "ipaddress", "interfaces"},
			validate: func(t *testing.T, result map[string]interface{}, warnings []string) {
				assert.Equal(t, "00:11:22:33:44:55", result["macaddress"])
				assert.Equal(t, "192.168.1.100", result["ipaddress"])
				_, hasIPv6 := result["ipaddressv6"]
				assert.False(t, hasIPv6, "ipaddressv6 should not be present when not found")
				assert.Empty(t, warnings)
			},
		},
		{
			name: "info with ipv6",
			info: &Info{
				MacAddress:  "00:11:22:33:44:55",
				IPAddress:   "192.168.1.100",
				IPAddressV6: utils.NewValue("fe80::1"),
				Interfaces:  []Interface{},
			},
			validate: func(t *testing.T, result map[string]interface{}, warnings []string) {
				assert.Equal(t, "fe80::1", result["ipaddressv6"])
				assert.Empty(t, warnings)
			},
		},
		{
			name: "info with interfaces",
			info: &Info{
				MacAddress:  "00:11:22:33:44:55",
				IPAddress:   "192.168.1.100",
				IPAddressV6: utils.NewErrorValue[string](ErrAddressNotFound),
				Interfaces: []Interface{
					{
						Name:        "eth0",
						IPv4:        []string{"192.168.1.100"},
						IPv6:        []string{"fe80::1"},
						IPv4Network: utils.NewValue("192.168.1.0/24"),
						IPv6Network: utils.NewValue("fe80::/64"),
						MacAddress:  utils.NewValue("00:11:22:33:44:55"),
					},
				},
			},
			validate: func(t *testing.T, result map[string]interface{}, warnings []string) {
				ifaces := result["interfaces"].([]interface{})
				require.Len(t, ifaces, 1)
				iface := ifaces[0].(map[string]interface{})
				assert.Equal(t, "eth0", iface["name"])
				assert.Equal(t, "192.168.1.0/24", iface["ipv4-network"])
				assert.Equal(t, "fe80::/64", iface["ipv6-network"])
				assert.Equal(t, "00:11:22:33:44:55", iface["macaddress"])
				assert.Empty(t, warnings)
			},
		},
		{
			name: "interface with missing optional fields",
			info: &Info{
				MacAddress:  "00:11:22:33:44:55",
				IPAddress:   "192.168.1.100",
				IPAddressV6: utils.NewErrorValue[string](ErrAddressNotFound),
				Interfaces: []Interface{
					{
						Name:        "eth0",
						IPv4:        []string{},
						IPv6:        []string{},
						IPv4Network: utils.NewErrorValue[string](ErrAddressNotFound),
						IPv6Network: utils.NewErrorValue[string](ErrAddressNotFound),
						MacAddress:  utils.NewErrorValue[string](ErrAddressNotFound),
					},
				},
			},
			validate: func(t *testing.T, result map[string]interface{}, warnings []string) {
				ifaces := result["interfaces"].([]interface{})
				require.Len(t, ifaces, 1)
				iface := ifaces[0].(map[string]interface{})
				assert.Equal(t, "eth0", iface["name"])
				_, hasIPv4Network := iface["ipv4-network"]
				_, hasIPv6Network := iface["ipv6-network"]
				_, hasMac := iface["macaddress"]
				assert.False(t, hasIPv4Network)
				assert.False(t, hasIPv6Network)
				assert.False(t, hasMac)
				assert.Empty(t, warnings)
			},
		},
		{
			name: "interface with error (not ErrAddressNotFound) generates warning",
			info: &Info{
				MacAddress:  "00:11:22:33:44:55",
				IPAddress:   "192.168.1.100",
				IPAddressV6: utils.NewErrorValue[string](errors.New("some ipv6 error")),
				Interfaces: []Interface{
					{
						Name:        "eth0",
						IPv4:        []string{},
						IPv6:        []string{},
						IPv4Network: utils.NewErrorValue[string](errors.New("some ipv4 network error")),
						IPv6Network: utils.NewErrorValue[string](ErrAddressNotFound),
						MacAddress:  utils.NewErrorValue[string](ErrAddressNotFound),
					},
				},
			},
			validate: func(t *testing.T, _ map[string]interface{}, warnings []string) {
				require.Len(t, warnings, 2)
				assert.Contains(t, warnings[0], "ipv4-network")
				assert.Contains(t, warnings[0], "some ipv4 network error")
				assert.Contains(t, warnings[1], "some ipv6 error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, warnings, err := tt.info.AsJSON()
			require.NoError(t, err)

			resultMap := result.(map[string]interface{})
			if tt.expectedKeys != nil {
				for _, key := range tt.expectedKeys {
					_, ok := resultMap[key]
					assert.True(t, ok, "expected key %s to be present", key)
				}
			}

			if tt.validate != nil {
				tt.validate(t, resultMap, warnings)
			}
		})
	}
}

func TestAsJSONMarshalling(t *testing.T) {
	info := &Info{
		MacAddress:  "00:11:22:33:44:55",
		IPAddress:   "192.168.1.100",
		IPAddressV6: utils.NewValue("fe80::1"),
		Interfaces: []Interface{
			{
				Name:        "eth0",
				IPv4:        []string{"192.168.1.100"},
				IPv6:        []string{"fe80::1"},
				IPv4Network: utils.NewValue("192.168.1.0/24"),
				IPv6Network: utils.NewValue("fe80::/64"),
				MacAddress:  utils.NewValue("00:11:22:33:44:55"),
			},
		},
	}

	marshallable, warnings, err := info.AsJSON()
	require.NoError(t, err)
	assert.Empty(t, warnings)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

	// Any change to this datastructure should be notified to the backend
	// team to ensure compatibility.
	type Network struct {
		Interfaces []struct {
			Ipv4        []string `json:"ipv4"`
			Ipv6        []string `json:"ipv6"`
			Ipv6Network string   `json:"ipv6-network"`
			Macaddress  string   `json:"macaddress"`
			Name        string   `json:"name"`
			Ipv4Network string   `json:"ipv4-network"`
		} `json:"interfaces"`
		Ipaddress   string `json:"ipaddress"`
		Ipaddressv6 string `json:"ipaddressv6"`
		Macaddress  string `json:"macaddress"`
	}

	decoder := json.NewDecoder(bytes.NewReader(marshalled))
	// do not ignore unknown fields
	decoder.DisallowUnknownFields()

	var decodedNetwork Network
	err = decoder.Decode(&decodedNetwork)
	require.NoError(t, err)

	// check that we read the full json
	require.False(t, decoder.More())

	assert.Equal(t, "192.168.1.100", decodedNetwork.Ipaddress)
	assert.Equal(t, "00:11:22:33:44:55", decodedNetwork.Macaddress)
	assert.Equal(t, "fe80::1", decodedNetwork.Ipaddressv6)

	require.Len(t, decodedNetwork.Interfaces, 1)
	iface := decodedNetwork.Interfaces[0]
	assert.Equal(t, "eth0", iface.Name)
	assert.Equal(t, []string{"192.168.1.100"}, iface.Ipv4)
	assert.Equal(t, []string{"fe80::1"}, iface.Ipv6)
	assert.Equal(t, "192.168.1.0/24", iface.Ipv4Network)
	assert.Equal(t, "fe80::/64", iface.Ipv6Network)
	assert.Equal(t, "00:11:22:33:44:55", iface.Macaddress)
}

func TestIfacesToJSON(t *testing.T) {
	tests := []struct {
		name             string
		interfaces       []Interface
		expectedLen      int
		expectedWarnings int
		validate         func(t *testing.T, result []interface{}, warnings []string)
	}{
		{
			name:        "empty interfaces",
			interfaces:  []Interface{},
			expectedLen: 0,
		},
		{
			name: "single interface with all fields",
			interfaces: []Interface{
				{
					Name:        "eth0",
					IPv4:        []string{"192.168.1.100", "192.168.1.101"},
					IPv6:        []string{"fe80::1", "fe80::2"},
					IPv4Network: utils.NewValue("192.168.1.0/24"),
					IPv6Network: utils.NewValue("fe80::/64"),
					MacAddress:  utils.NewValue("00:11:22:33:44:55"),
				},
			},
			expectedLen: 1,
			validate: func(t *testing.T, result []interface{}, warnings []string) {
				iface := result[0].(map[string]interface{})
				assert.Equal(t, "eth0", iface["name"])
				assert.Equal(t, "192.168.1.0/24", iface["ipv4-network"])
				assert.Equal(t, "fe80::/64", iface["ipv6-network"])
				assert.Equal(t, "00:11:22:33:44:55", iface["macaddress"])
				assert.Empty(t, warnings)
			},
		},
		{
			name: "interface with ErrAddressNotFound does not generate warnings",
			interfaces: []Interface{
				{
					Name:        "eth0",
					IPv4:        []string{},
					IPv6:        []string{},
					IPv4Network: utils.NewErrorValue[string](ErrAddressNotFound),
					IPv6Network: utils.NewErrorValue[string](ErrAddressNotFound),
					MacAddress:  utils.NewErrorValue[string](ErrAddressNotFound),
				},
			},
			expectedLen:      1,
			expectedWarnings: 0,
		},
		{
			name: "interface with other errors generates warnings",
			interfaces: []Interface{
				{
					Name:        "eth0",
					IPv4:        []string{},
					IPv6:        []string{},
					IPv4Network: utils.NewErrorValue[string](errors.New("ipv4 error")),
					IPv6Network: utils.NewErrorValue[string](errors.New("ipv6 error")),
					MacAddress:  utils.NewErrorValue[string](errors.New("mac error")),
				},
			},
			expectedLen:      1,
			expectedWarnings: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, warnings := ifacesToJSON(tt.interfaces)
			resultSlice := result.([]interface{})
			assert.Len(t, resultSlice, tt.expectedLen)
			assert.Len(t, warnings, tt.expectedWarnings)

			if tt.validate != nil {
				tt.validate(t, resultSlice, warnings)
			}
		})
	}
}

// createIPAddrAddr creates a *net.IPAddr address from an IP string
func createIPAddrAddr(ipStr string) *net.IPAddr {
	return &net.IPAddr{IP: net.ParseIP(ipStr)}
}

func TestExternalIpv6AddressWithIPAddr(t *testing.T) {
	// Test that *net.IPAddr type is handled correctly in type switch
	ifaces := []networkInterface{
		&mockNetworkInterface{
			name:  "eth0",
			flags: net.FlagUp,
			addrs: []net.Addr{
				createIPAddrAddr("fe80::1"),
			},
		},
	}
	setMockInterfaces(t, ifaces, nil)

	ip, err := externalIpv6Address()
	require.NoError(t, err)
	assert.Equal(t, "fe80::1", ip)
}

func TestExternalIpv6AddressAddrsError(t *testing.T) {
	// Test that Addrs() error is returned
	ifaces := []networkInterface{
		&mockNetworkInterface{
			name:     "eth0",
			flags:    net.FlagUp,
			addrsErr: errors.New("addrs error"),
		},
	}
	setMockInterfaces(t, ifaces, nil)

	_, err := externalIpv6Address()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "addrs error")
}

func TestExternalIPAddressWithIPAddr(t *testing.T) {
	// Test that *net.IPAddr type is handled correctly in type switch
	ifaces := []networkInterface{
		&mockNetworkInterface{
			name:  "eth0",
			flags: net.FlagUp,
			addrs: []net.Addr{
				createIPAddrAddr("192.168.1.100"),
			},
		},
	}
	setMockInterfaces(t, ifaces, nil)

	ip, err := externalIPAddress()
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.100", ip)
}

func TestExternalIPAddressAddrsError(t *testing.T) {
	// Test that Addrs() error is returned
	ifaces := []networkInterface{
		&mockNetworkInterface{
			name:     "eth0",
			flags:    net.FlagUp,
			addrsErr: errors.New("addrs error"),
		},
	}
	setMockInterfaces(t, ifaces, nil)

	_, err := externalIPAddress()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "addrs error")
}

func TestMacAddressWithIPAddr(t *testing.T) {
	// Test that *net.IPAddr type is handled correctly in type switch
	hw, _ := net.ParseMAC("00:11:22:33:44:55")
	ifaces := []networkInterface{
		&mockNetworkInterface{
			name:         "eth0",
			flags:        net.FlagUp,
			hardwareAddr: hw,
			addrs: []net.Addr{
				createIPAddrAddr("192.168.1.100"),
			},
		},
	}
	setMockInterfaces(t, ifaces, nil)

	mac, err := macAddress()
	require.NoError(t, err)
	assert.Equal(t, "00:11:22:33:44:55", mac)
}

func TestMacAddressAddrsError(t *testing.T) {
	// Test that Addrs() error is returned
	ifaces := []networkInterface{
		&mockNetworkInterface{
			name:     "eth0",
			flags:    net.FlagUp,
			addrsErr: errors.New("addrs error"),
		},
	}
	setMockInterfaces(t, ifaces, nil)

	_, err := macAddress()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "addrs error")
}

func TestGetNetworkInfoExternalIpv6Error(t *testing.T) {
	// Test that externalIpv6Address error is propagated
	// We need macAddress and externalIPAddress to succeed, but externalIpv6Address to fail
	// This requires a stateful mock that returns different results on different calls

	// Create a counter to track calls
	callCount := 0
	original := getInterfaces

	hw, _ := net.ParseMAC("00:11:22:33:44:55")
	normalIface := &mockNetworkInterface{
		name:         "eth0",
		flags:        net.FlagUp,
		hardwareAddr: hw,
		addrs: []net.Addr{
			createIPNetAddr("192.168.1.100/24"),
		},
	}

	getInterfaces = func() ([]networkInterface, error) {
		callCount++
		// First two calls (macAddress and externalIPAddress) succeed
		// Third call (externalIpv6Address) fails
		if callCount <= 2 {
			return []networkInterface{normalIface}, nil
		}
		return nil, errors.New("ipv6 lookup error")
	}
	t.Cleanup(func() {
		getInterfaces = original
	})

	_, err := getNetworkInfo()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ipv6 lookup error")
}

func TestGetNetworkInfoExternalIPAddressError(t *testing.T) {
	// Test that externalIPAddress error is propagated after macAddress succeeds
	// We need macAddress to succeed but externalIPAddress to fail

	callCount := 0
	original := getInterfaces

	hw, _ := net.ParseMAC("00:11:22:33:44:55")
	normalIface := &mockNetworkInterface{
		name:         "eth0",
		flags:        net.FlagUp,
		hardwareAddr: hw,
		addrs: []net.Addr{
			createIPNetAddr("192.168.1.100/24"),
		},
	}

	getInterfaces = func() ([]networkInterface, error) {
		callCount++
		// First call (macAddress) succeeds
		// Second call (externalIPAddress) fails
		if callCount == 1 {
			return []networkInterface{normalIface}, nil
		}
		return nil, errors.New("ip lookup error")
	}
	t.Cleanup(func() {
		getInterfaces = original
	})

	_, err := getNetworkInfo()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ip lookup error")
}

func TestGetMultiNetworkInfoWithNilIP(t *testing.T) {
	// Test that nil IP from ParseCIDR is handled (continues to next addr)
	// This happens when addr.String() returns something that ParseCIDR can't parse

	// Create an IPNet with nil IP which will result in nil IP after ParseCIDR
	ifaces := []networkInterface{
		&mockNetworkInterface{
			name:  "eth0",
			flags: net.FlagUp,
			addrs: []net.Addr{
				&net.IPNet{IP: nil, Mask: nil}, // This will result in nil IP after ParseCIDR
				createIPNetAddr("192.168.1.100/24"),
			},
			hardwareAddr: func() net.HardwareAddr {
				hw, _ := net.ParseMAC("00:11:22:33:44:55")
				return hw
			}(),
		},
	}
	setMockInterfaces(t, ifaces, nil)

	result, err := getMultiNetworkInfo()
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"192.168.1.100"}, result[0].IPv4)
}

func TestRealNetworkInterfaceMethods(t *testing.T) {
	// Test realNetworkInterface methods directly
	realIface := &realNetworkInterface{
		iface: net.Interface{
			Index:        1,
			MTU:          1500,
			Name:         "test0",
			HardwareAddr: net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
			Flags:        net.FlagUp | net.FlagBroadcast,
		},
	}

	assert.Equal(t, "test0", realIface.GetName())
	assert.Equal(t, net.FlagUp|net.FlagBroadcast, realIface.GetFlags())
	assert.Equal(t, "00:11:22:33:44:55", realIface.GetHardwareAddr().String())

	// Addrs() may or may not return an error depending on the system
	// We're just testing that the method can be called
	_, _ = realIface.Addrs()
}

// Helpers

func assertIPv4(t *testing.T, addr string) {
	t.Helper()
	res := net.ParseIP(addr)
	if assert.NotNil(t, res, "not a valid ipv4 address: %s", addr) {
		assert.NotNil(t, res.To4())
	}
}

func assertIPv6(t *testing.T, addr string) {
	t.Helper()
	res := net.ParseIP(addr)
	if assert.NotNil(t, res, "not a valid ipv6 address: %s", addr) {
		assert.NotNil(t, res.To16())
	}
}

func assertValueIPv6(t *testing.T, addr utils.Value[string]) {
	t.Helper()
	if ipv6, err := addr.Value(); err == nil {
		assertIPv6(t, ipv6)
	} else {
		assert.ErrorIs(t, err, ErrAddressNotFound)
	}
}

func assertMac(t *testing.T, addr string) {
	t.Helper()
	if addr != "" {
		_, err := net.ParseMAC(addr)
		assert.NoErrorf(t, err, "addr %s", addr)
	}
}

func assertValueMac(t *testing.T, addr utils.Value[string]) {
	t.Helper()
	if mac, err := addr.Value(); err == nil {
		assertMac(t, mac)
	} else {
		assert.ErrorIs(t, err, ErrAddressNotFound)
	}
}

func assertCIDR(t *testing.T, addr string) {
	t.Helper()
	_, _, err := net.ParseCIDR(addr)
	assert.NoError(t, err)
}

func assertValueCIDR(t *testing.T, addr utils.Value[string]) {
	t.Helper()
	if addr, err := addr.Value(); err == nil {
		assertCIDR(t, addr)
	} else {
		assert.ErrorIs(t, err, ErrAddressNotFound)
	}
}
