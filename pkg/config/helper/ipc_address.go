// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"fmt"
	"net"
	"strconv"

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
