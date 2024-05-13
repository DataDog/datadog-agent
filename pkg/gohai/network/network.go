// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package network regroups collecting information about the network interfaces
package network

import (
	"errors"
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

// ErrAddressNotFound means no such address could be found
var ErrAddressNotFound = errors.New("address not found")

// Interface holds information about a specific interface
type Interface struct {
	// Name is the name of the interface
	Name string `json:"name"`
	// IPv4Network is the ipv4 address for the host
	IPv4Network utils.Value[string] `json:"ipv4-network"`
	// IPv6Network is the ipv6 address for the host
	IPv6Network utils.Value[string] `json:"ipv6-network"`
	// MacAddress is the mac address for the host
	MacAddress utils.Value[string] `json:"macaddress"`
	// IPv4 is the list of IPv4 addresses for the interface
	IPv4 []string `json:"ipv4"`
	// IPv4 is the list of IPv6 addresses for the interface
	IPv6 []string `json:"ipv6"`
}

// Info holds network metadata about the host
type Info struct {
	// Interfaces is the list of interfaces which are up
	Interfaces []Interface `json:"interfaces"`
	// MacAddress is a mac address of the host
	MacAddress string `json:"macaddress"`
	// IPAddress is an IPv4 address of the host
	IPAddress string `json:"ipaddress"`
	// IPAddressV6 is an IPv6 address of the host
	IPAddressV6 utils.Value[string] `json:"ipaddressv6"`
}

// CollectInfo collects the network information.
func CollectInfo() (*Info, error) {
	info, err := getNetworkInfo()
	if err != nil {
		return nil, err
	}

	interfaces, err := getMultiNetworkInfo()
	if err == nil && len(interfaces) > 0 {
		info.Interfaces = interfaces
	}
	return info, err
}

func ifacesToJSON(ifaces []Interface) (interface{}, []string) {
	ret := make([]interface{}, len(ifaces))
	warnings := []string{}

	for idx, iface := range ifaces {
		ifaceJSON := map[string]interface{}{
			"name": iface.Name,
			"ipv4": iface.IPv4,
			"ipv6": iface.IPv6,
		}

		values := map[string]utils.Value[string]{
			"ipv4-network": iface.IPv4Network,
			"ipv6-network": iface.IPv6Network,
			"macaddress":   iface.MacAddress,
		}
		for key, value := range values {
			if val, err := value.Value(); err == nil {
				ifaceJSON[key] = val
			} else if !errors.Is(err, ErrAddressNotFound) {
				warnings = append(warnings, fmt.Sprintf("%s: %s: %v", iface.Name, key, err))
			}
		}

		ret[idx] = ifaceJSON
	}

	return ret, warnings
}

// AsJSON returns an interface which can be marshalled to a JSON and contains the value of non-errored fields.
func (netInfo *Info) AsJSON() (interface{}, []string, error) {
	interfaces, warnings := ifacesToJSON(netInfo.Interfaces)
	ret := map[string]interface{}{
		"macaddress": netInfo.MacAddress,
		"ipaddress":  netInfo.IPAddress,
		"interfaces": interfaces,
	}

	if ipv6, err := netInfo.IPAddressV6.Value(); err == nil {
		ret["ipaddressv6"] = ipv6
	} else if !errors.Is(err, ErrAddressNotFound) {
		warnings = append(warnings, err.Error())
	}

	return ret, warnings, nil
}

func getMultiNetworkInfo() ([]Interface, error) {
	multiNetworkInfo := []Interface{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return multiNetworkInfo, err
	}

	for _, iface := range ifaces {
		defaultAddrErr := fmt.Errorf("%s: %w", iface.Name, ErrAddressNotFound)
		_iface := Interface{
			IPv6Network: utils.NewErrorValue[string](defaultAddrErr),
			IPv4Network: utils.NewErrorValue[string](defaultAddrErr),
			MacAddress:  utils.NewErrorValue[string](defaultAddrErr),
			IPv4:        []string{},
			IPv6:        []string{},
			Name:        iface.Name,
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			// interface down or loopback interface
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			// skip this interface but try the next
			continue
		}
		for _, addr := range addrs {
			ip, network, _ := net.ParseCIDR(addr.String())
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip.To4() == nil {
				_iface.IPv6 = append(_iface.IPv6, ip.String())
				_iface.IPv6Network = utils.NewValue(network.String())
			} else {
				_iface.IPv4 = append(_iface.IPv4, ip.String())
				_iface.IPv4Network = utils.NewValue(network.String())
			}
			if len(iface.HardwareAddr.String()) > 0 {
				_iface.MacAddress = utils.NewValue(iface.HardwareAddr.String())
			}
		}
		multiNetworkInfo = append(multiNetworkInfo, _iface)
	}

	return multiNetworkInfo, nil
}

func externalIpv6Address() (string, error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			// interface down or loopback interface
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip.To4() != nil {
				// ipv4 address
				continue
			}
			return ip.String(), nil
		}
	}

	// We don't return an error if no IPv6 interface has been found. Indeed,
	// some orgs just don't have IPv6 enabled. If there's a network error, it
	// will pop out when getting the Mac address and/or the IPv4 address
	// (before this function's call; see network.go -> getNetworkInfo())
	return "", nil
}

func externalIPAddress() (string, error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			// interface down or loopback interface
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				// not an ipv4 address
				continue
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("not connected to the network")
}

func macAddress() (string, error) {
	ifaces, err := net.Interfaces()

	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			// interface down or loopback interface
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			return iface.HardwareAddr.String(), nil
		}
	}
	return "", errors.New("not connected to the network")
}

func getNetworkInfo() (*Info, error) {
	macaddress, err := macAddress()
	if err != nil {
		return nil, err
	}

	ipAddress, err := externalIPAddress()
	if err != nil {
		return nil, err
	}

	ipAddressV6, err := externalIpv6Address()
	if err != nil {
		return nil, err
	}

	networkInfo := &Info{
		MacAddress: macaddress,
		IPAddress:  ipAddress,
	}

	// We append an IPv6 address to the payload only if IPv6 is enabled
	if ipAddressV6 != "" {
		networkInfo.IPAddressV6 = utils.NewValue(ipAddressV6)
	} else {
		networkInfo.IPAddressV6 = utils.NewErrorValue[string](ErrAddressNotFound)
	}

	return networkInfo, nil
}
