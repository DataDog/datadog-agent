// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package listener supports platform specific net.Listener implementations for IPC Server creation
package listener

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
	"github.com/mdlayher/vsock"
)

// GetIPCAddressPort returns a listening connection
func GetIPCAddressPort() (string, error) {
	address, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(address, pkgconfigsetup.GetIPCPort()), nil
}

// GetListener returns a listening connection
func GetListener(address string) (net.Listener, error) {
	if vsockAddr := pkgconfigsetup.Datadog().GetString("vsock_addr"); vsockAddr != "" {
		_, sPort, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		port, err := strconv.ParseUint(sPort, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid port for vsock listener: %v", err)
		}

		cid, err := socket.ParseVSockAddress(vsockAddr)
		if err != nil {
			return nil, err
		}

		listener, err := vsock.ListenContextID(cid, uint32(port), &vsock.Config{})
		return listener, err
	}

	// this is used by both the CMDServer as well as the IPCServer so we cannot rely on the config value
	if strings.Contains(address, "/") {
		// currently only unix sockets are supported
		return platformSpecificListener(address)
	}

	return net.Listen("tcp", address)
}

// GetIPCServerPath returns whether the IPC server is enabled, and if so its host:port or unix socket path
func GetIPCServerPath() (string, bool) {
	if pkgconfigsetup.Datadog().GetBool("agent_ipc.use_socket") {
		if !hasPlatformSupport() {
			return "", false
		}

		socketPath := pkgconfigsetup.Datadog().GetString("agent_ipc.socket_path")
		return socketPath, true
	}

	ipcServerPort := pkgconfigsetup.Datadog().GetInt("agent_ipc.port")
	if ipcServerPort == 0 {
		return "", false
	}

	ipcServerHost := pkgconfigsetup.Datadog().GetString("agent_ipc.host")
	ipcServerHostPort := net.JoinHostPort(ipcServerHost, strconv.Itoa(ipcServerPort))

	return ipcServerHostPort, true
}
