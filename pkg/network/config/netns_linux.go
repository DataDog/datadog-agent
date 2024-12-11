// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"

	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// GetRootNetNs returns the network namespace to use for creating, e.g., netlink sockets
//
// This will be the host's default network namespace if network_config.enable_root_netns
// is set to true (the default); otherwise this will be the default network namespace
// for the current process
func (c *Config) GetRootNetNs() (netns.NsHandle, error) {
	if !c.EnableRootNetNs {
		return netns.GetFromPid(os.Getpid())
	}

	// get the root network namespace
	return kernel.GetRootNetNamespace(c.ProcRoot)
}
