// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package helper

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// GetGUIAddress returns the GUI's loopback IP address and port.
func GetGUIAddress(config model.Reader) (string, error) {
	host, err := system.IsLocalAddress(config.GetString("GUI_host"))
	if err != nil {
		return "", err
	}
	// Use the same address family for the listener and browser launchers,
	// independently of how each resolves localhost.
	if host == "localhost" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, config.GetString("GUI_port")), nil
}
