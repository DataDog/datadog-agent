// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"net"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// newListener creates a listening connection
func newListener() (net.Listener, error) {
	address, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}
	return net.Listen("tcp", fmt.Sprintf("%v:%v", address, pkgconfigsetup.Datadog().GetInt("security_agent.cmd_port")))
}
