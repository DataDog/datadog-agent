// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"net"
	"strconv"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/mdlayher/vsock"
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
	if pkgconfigsetup.Datadog().GetString("vsock_addr") != "" {
		port := pkgconfigsetup.Datadog().GetInt("cmd_port")
		listener, err := vsock.Listen(uint32(port), &vsock.Config{})
		return listener, err
	}
	listener, err := net.Listen("tcp", address)
	return listener, err
}

// getIPCServerAddressPort returns whether the IPC server is enabled, and if so its host and host:port
func getIPCServerAddressPort() (string, string, bool) {
	ipcServerPort := pkgconfigsetup.Datadog().GetInt("agent_ipc.port")
	if ipcServerPort == 0 {
		return "", "", false
	}

	ipcServerHost := pkgconfigsetup.Datadog().GetString("agent_ipc.host")
	ipcServerHostPort := net.JoinHostPort(ipcServerHost, strconv.Itoa(ipcServerPort))

	return ipcServerHost, ipcServerHostPort, true
}
