// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"fmt"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// GetIPCAddress returns the IPC address or an error if the address is not local
func GetIPCAddress(config pkgconfigmodel.Reader) (string, error) {
	var key string
	// ipc_address is deprecated in favor of cmd_host, but we still need to support it
	// if it is set, use it, otherwise use cmd_host
	if config.IsSet("ipc_address") {
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
func GetIPCPort() string {
	return Datadog().GetString("cmd_port")
}
