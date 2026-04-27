// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"net"
	"strconv"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultProcessCmdPort = 6162

// GetProcessAPIAddressPort returns the API endpoint of the process agent.
func GetProcessAPIAddressPort(config pkgconfigmodel.Reader) (string, error) {
	address, err := pkgconfigsetup.GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	port := config.GetInt("process_config.cmd_port")
	if port <= 0 {
		log.Warnf("Invalid process_config.cmd_port -- %d, using default port %d", port, defaultProcessCmdPort)
		port = defaultProcessCmdPort
	}

	return net.JoinHostPort(address, strconv.Itoa(port)), nil
}
