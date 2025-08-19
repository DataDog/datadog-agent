// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package network

import (
	"bytes"
	"encoding/json"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestAsJSON(t *testing.T) {
	netInfo, err := CollectInfo()
	require.NoError(t, err)

	marshallable, _, err := netInfo.AsJSON()
	require.NoError(t, err)

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

	assertIPv4(t, decodedNetwork.Ipaddress)
	assertMac(t, decodedNetwork.Macaddress)
	if decodedNetwork.Ipaddressv6 != "" {
		assertIPv6(t, decodedNetwork.Ipaddressv6)
	}

	require.NotEmpty(t, decodedNetwork.Interfaces)
	for _, iface := range decodedNetwork.Interfaces {
		assert.NotEmpty(t, iface.Name)

		if iface.Ipv4Network != "" {
			assertCIDR(t, iface.Ipv4Network)
		}
		if iface.Ipv6Network != "" {
			assertCIDR(t, iface.Ipv6Network)
		}

		// Some interfaces don't have MacAddresses
		if iface.Macaddress != "" {
			assertMac(t, iface.Macaddress)
		}

		for _, addr := range iface.Ipv4 {
			assertIPv4(t, addr)
		}
		for _, addr := range iface.Ipv6 {
			assertIPv6(t, addr)
		}
	}
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
	_, err := net.ParseMAC(addr)
	assert.NoError(t, err)
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
