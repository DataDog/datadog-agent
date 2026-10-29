// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// GetIPCAddress returns the IPC address or an error if the address is not local
func GetIPCAddress(config model.Reader) (string, error) {
	var key string
	// ipc_address is deprecated in favor of cmd_host, but we still need to support it
	// if it is set, use it, otherwise use cmd_host
	if config.IsConfigured("ipc_address") {
		log.Warn("ipc_address is deprecated, use cmd_host instead")
		key = "ipc_address"
	} else {
		key = "cmd_host"
	}

	address, err := system.IsLocalAddress(config.GetString(key))
	if err != nil {
		return "", fmt.Errorf("%s: %s", key, err)
	}
	return address, nil
}

// apmDebugHostFallbackOnce keeps the fallback warning to a single line.
// Trace telemetry calls APMDebugHost every 45s, and status calls it on every refresh.
var apmDebugHostFallbackOnce sync.Once

func warnAPMDebugHostFallback(format string, args ...interface{}) {
	apmDebugHostFallbackOnce.Do(func() {
		log.Warnf(format, args...)
	})
}

// APMDebugHost returns the address the trace-agent debug server binds.
// The IPC certificate only names 127.0.0.1 and ::1. localhost and any other
// address use 127.0.0.1 so TLS hostname verification succeeds.
func APMDebugHost(config model.Reader) string {
	const ipv4Loopback = "127.0.0.1"

	address, err := GetIPCAddress(config)
	if err != nil {
		warnAPMDebugHostFallback("APM debug server will use %s: %s", ipv4Loopback, err)
		return ipv4Loopback
	}
	if host, ok := ipcCertHost(address); ok {
		return host
	}
	if address != "" && address != "localhost" {
		warnAPMDebugHostFallback("APM debug server will use %s: %s is not covered by the IPC certificate", ipv4Loopback, address)
	}
	return ipv4Loopback
}

// ipcCertHost returns the canonical 127.0.0.1 or ::1 when address is that IP.
// localhost is also on the certificate, but dialing that name tries ::1, so it is rejected here.
func ipcCertHost(address string) (string, bool) {
	ip := net.ParseIP(address)
	if ip == nil {
		return "", false
	}
	if ip.Equal(net.ParseIP("127.0.0.1")) {
		return "127.0.0.1", true
	}
	if ip.Equal(net.IPv6loopback) {
		return "::1", true
	}
	return "", false
}

// GetIPCPort returns the IPC port
func GetIPCPort(config model.Reader) string {
	return config.GetString("cmd_port")
}

// GetProcessAPIAddressPort returns the API endpoint of the process agent
func GetProcessAPIAddressPort(config model.Reader) (string, error) {
	address, err := GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	port := config.GetInt("process_config.cmd_port")
	if port <= 0 {
		log.Warnf("Invalid process_config.cmd_port -- %d, using default port %d", port, constants.DefaultProcessCmdPort)
		port = constants.DefaultProcessCmdPort
	}

	addrPort := net.JoinHostPort(address, strconv.Itoa(port))
	return addrPort, nil
}

// GetSecurityAgentAPIAddressPort returns the API endpoint of the security agent
func GetSecurityAgentAPIAddressPort(config model.Reader) (string, error) {
	address, err := GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	port := config.GetInt("security_agent.cmd_port")
	if port <= 0 {
		log.Warnf("Invalid security.cmd_port -- %d, using default port %d", port, constants.DefaultSecurityAgentCmdPort)
		port = constants.DefaultProcessCmdPort
	}

	addrPort := net.JoinHostPort(address, strconv.Itoa(port))
	return addrPort, nil
}
