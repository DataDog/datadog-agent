// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package network regroups collecting information about the network interfaces
package network

import (
	"errors"
	"net"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

var ErrAddressNotFound = errors.New("address not found for the network interface")

// Network holds network metadata about the host
type Network struct {
	// IpAddress is the ipv4 address for the host
	IpAddress string
	// IpAddressv6 is the ipv6 address for the host
	IpAddressv6 string
	// MacAddress is the macaddress for the host
	MacAddress string

	// TODO: the collect method also returns metadata about interfaces. They should be added to this struct.
	// Since it would require even more cleanup we'll do it in another PR when needed.
}

type Interface struct {
	IPv6Network utils.Value[string] `json:"ipv6-network"`
	IPv4Network utils.Value[string] `json:"ipv4-network"`
	MacAddress  utils.Value[string] `json:"macaddress"`
	IPv4        []string            `json:"ipv4"`
	IPv6        []string            `json:"ipv6"`
	Name        string              `json:"name"`
}

type Info struct {
	// interfaces utils.Value[]
}

// Collect collects the Network information.
// Returns an object which can be converted to a JSON or an error if nothing could be collected.
// Tries to collect as much information as possible.
func (network *Network) Collect() (result interface{}, err error) {
	result, err = getNetworkInfo()
	if err != nil {
		return
	}

	interfaces, err := getMultiNetworkInfo()
	if err == nil && len(interfaces) > 0 {
		interfaceMap, ok := result.(map[string]interface{})
		if !ok {
			return
		}
		interfaceMap["interfaces"] = interfaces
	}
	return
}

// Get returns a Network struct already initialized, a list of warnings and an error. The method will try to collect as much
// metadata as possible, an error is returned if nothing could be collected. The list of warnings contains errors if
// some metadata could not be collected.
func Get() (*Network, []string, error) {
	networkInfo, err := getNetworkInfo()
	if err != nil {
		return nil, nil, err
	}

	return &Network{
		IpAddress:   utils.GetStringInterface(networkInfo, "ipaddress"),
		IpAddressv6: utils.GetStringInterface(networkInfo, "ipaddressv6"),
		MacAddress:  utils.GetStringInterface(networkInfo, "macaddress"),
	}, nil, nil
}

func getMultiNetworkInfo() ([]Interface, error) {
	multiNetworkInfo := []Interface{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return multiNetworkInfo, err
	}

	for _, iface := range ifaces {
		_iface := Interface{
			IPv6Network: utils.NewErrorValue[string](ErrAddressNotFound),
			IPv4Network: utils.NewErrorValue[string](ErrAddressNotFound),
			MacAddress:  utils.NewErrorValue[string](ErrAddressNotFound),
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

func getNetworkInfo() (networkInfo map[string]interface{}, err error) {
	networkInfo = make(map[string]interface{})

	macaddress, err := macAddress()
	if err != nil {
		return networkInfo, err
	}
	networkInfo["macaddress"] = macaddress

	ipAddress, err := externalIPAddress()
	if err != nil {
		return networkInfo, err
	}
	networkInfo["ipaddress"] = ipAddress

	ipAddressV6, err := externalIpv6Address()
	if err != nil {
		return networkInfo, err
	}
	// We append an IPv6 address to the payload only if IPv6 is enabled
	if ipAddressV6 != "" {
		networkInfo["ipaddressv6"] = ipAddressV6
	}

	return
}
