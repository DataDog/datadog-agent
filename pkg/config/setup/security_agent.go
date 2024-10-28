// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"net"
	"strconv"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DefaultSecurityAgentCmdPort is the default port used by security-agent to run a runtime settings server
const DefaultSecurityAgentCmdPort = 5010

// GetSecurityAgentAPIAddressPort returns the API endpoint of the security agent
func GetSecurityAgentAPIAddressPort(config pkgconfigmodel.Reader) (string, error) {
	address, err := GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	port := config.GetInt("security_agent.cmd_port")
	if port <= 0 {
		log.Warnf("Invalid security.cmd_port -- %d, using default port %d", port, DefaultSecurityAgentCmdPort)
		port = DefaultProcessCmdPort
	}

	addrPort := net.JoinHostPort(address, strconv.Itoa(port))
	return addrPort, nil
}
