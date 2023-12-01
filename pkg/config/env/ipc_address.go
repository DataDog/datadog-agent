// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"fmt"
	"net"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// IsLocalAddress returns the given address if it is local or an error if it is not
func IsLocalAddress(address string) (string, error) {
	if address == "localhost" {
		return address, nil
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return "", fmt.Errorf("address was set to an invalid IP address: %s", address)
	}
	for _, cidr := range []string{
		"127.0.0.0/8", // IPv4 loopback
		"::1/128",     // IPv6 loopback
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		if block.Contains(ip) {
			return address, nil
		}
	}
	return "", fmt.Errorf("address was set to a non-loopback IP address: %s", address)
}

// GetIPCAddress returns the IPC address or an error if the address is not local
func GetIPCAddress(cfg pkgconfigmodel.Reader) (string, error) {
	address, err := IsLocalAddress(cfg.GetString("ipc_address"))
	if err != nil {
		return "", fmt.Errorf("ipc_address: %s", err)
	}
	return address, nil
}
