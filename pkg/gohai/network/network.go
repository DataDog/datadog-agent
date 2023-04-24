// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package network regroups collecting information about the network interfaces
package network

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
