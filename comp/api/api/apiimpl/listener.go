// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
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
	if vsockAddr := pkgconfigsetup.Datadog().GetString("vsock_addr"); vsockAddr != "" {
		_, sPort, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		port, err := strconv.Atoi(sPort)
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

	if strings.HasSuffix(address, ".socket") {
		return net.Listen("unix", address)
	}

	return net.Listen("tcp", address)
}

// getIPCServerAddressPort returns whether the IPC server is enabled, and if so its host:port or unix socket path
func getIPCServerAddressPort() (string, bool) {
	if pkgconfigsetup.Datadog().GetBool("agent_ipc.use_socket") {
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
