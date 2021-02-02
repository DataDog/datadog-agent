// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"net"

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
func getListener() (net.Listener, error) {
	address, err := getIPCAddressPort()
	if err != nil {
		return nil, err
	}
	return net.Listen("tcp", address)
}
