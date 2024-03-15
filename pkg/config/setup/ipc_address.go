// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"fmt"
	"net"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
func GetIPCAddress(config pkgconfigmodel.Reader) (string, error) {
	return getIPCAddress(config)
}

// GetIPCPort returns the IPC port
func GetIPCPort() string {
	return Datadog.GetString("cmd_port")
}

func getIPCAddress(cfg pkgconfigmodel.Reader) (string, error) {
	var key string
	// ipc_address is deprecated in favor of cmd_host, but we still need to support it
	// if it is set, use it, otherwise use cmd_host
	if cfg.IsSet("ipc_address") {
		log.Warn("ipc_address is deprecated, use cmd_host instead")
		key = "ipc_address"
	} else {
		key = "cmd_host"
	}

	address, err := IsLocalAddress(cfg.GetString(key))
	if err != nil {
		return "", fmt.Errorf("%s: %s", key, err)
	}
	return address, nil
}
