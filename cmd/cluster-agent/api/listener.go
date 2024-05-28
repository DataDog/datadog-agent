// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// getListener returns a listening connection
func getListener() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", config.Datadog().GetInt("cluster_agent.cmd_port")))
}
