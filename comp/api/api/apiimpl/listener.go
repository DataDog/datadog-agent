// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"net"
	"strconv"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// getIPCAddressPort returns a listening connection
func getIPCAddressPort() (string, error) {
	address, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(address, pkgconfigsetup.GetIPCPort()), nil
}

// getListener returns a listening connection
func getListener(address string) (net.Listener, error) {
	// if the address is an IP address, return a TCP listener otherwise try it as a unix socket
	if ipAddr := net.ParseIP(address); ipAddr != nil {
		return net.Listen("tcp", ipAddr)
	}
	return net.Listen("unix", address)
}

// returns whether the IPC server is enabled, and if so its host and host:port
func getIPCServerAddressPort() (string, bool) {
	if pkgconfigsetup.Datadog().GetBool("agent_ipc.use_uds") {
		socketPath := pkgconfigsetup.Datadog().GetString("agent_ipc.socket_path")
		return socketPath + "/agent_ipc.socket", true
	}

	ipcServerPort := pkgconfigsetup.Datadog().GetInt("agent_ipc.port")
	if ipcServerPort == 0 {
		return "", false
	}

	ipcServerHost := pkgconfigsetup.Datadog().GetString("agent_ipc.host")
	ipcServerHostPort := net.JoinHostPort(ipcServerHost, strconv.Itoa(ipcServerPort))

	return ipcServerHostPort, true
}
