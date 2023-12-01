// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"net"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// getIPCAddressPort returns a listening connection
func getIPCAddressPort() (string, error) {
	address, err := config.GetIPCAddress()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v:%v", address, config.Datadog.GetInt("cmd_port")), nil
}

// getListener returns a listening connection
func getListener(address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}

// returns the host and host:port of the IPC server, and whether it is enabled
func getIPCServerAddressPort() (string, string, bool) {
	ipcServerHost := config.Datadog.GetString("agent_ipc_host")
	ipcServerPort := config.Datadog.GetInt("agent_ipc_port")
	ipcServerHostPort := net.JoinHostPort(ipcServerHost, strconv.Itoa(ipcServerPort))
	ipcServerEnabled := ipcServerPort != 0

	return ipcServerHost, ipcServerHostPort, ipcServerEnabled
}
